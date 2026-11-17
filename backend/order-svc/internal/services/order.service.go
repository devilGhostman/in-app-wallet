package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"order-svc/internal/config"
	"order-svc/internal/dto"
	"order-svc/internal/model"
	"order-svc/internal/repository"
	"order-svc/pkg/response"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type OrderService struct {
	repo          repository.OrderRepository
	limits        config.LimitsConfig
	userIDPattern *regexp.Regexp
	log           *zap.Logger
}

func NewOrderService(repo repository.OrderRepository, limits config.LimitsConfig, userIDPattern string, log *zap.Logger) *OrderService {
	var re *regexp.Regexp
	if strings.TrimSpace(userIDPattern) != "" {
		re = regexp.MustCompile(userIDPattern)
	}
	return &OrderService{repo: repo, limits: limits, userIDPattern: re, log: log}
}

func (s *OrderService) Create(ctx context.Context, appID string, req dto.CreateOrderRequest) (dto.OrderResponse, error) {
	appID, userID, amount, key, meta, err := s.validateCreate(appID, req.UserID, req.Amount, req.IdempotencyKey, req.Metadata)
	if err != nil {
		return dto.OrderResponse{}, err
	}
	if existing, err := s.repo.GetByIdempotency(ctx, appID, key); err == nil {
		if existing.Type != model.TypePurchase || existing.UserID != userID || existing.Amount != amount {
			return dto.OrderResponse{}, response.Conflict(response.CodeKeyPayloadMismatch, "idempotency key reused with different payload")
		}
		return toOrderResponse(existing), nil
	} else if !errors.Is(err, repository.ErrOrderNotFound) {
		return dto.OrderResponse{}, response.Internal("failed to load order", err)
	}

	now := time.Now().UTC()
	order := model.Order{
		ID:             uuid.New(),
		AppID:          appID,
		UserID:         userID,
		Amount:         amount,
		Type:           model.TypePurchase,
		Status:         model.StatusPending,
		IdempotencyKey: key,
		Metadata:       meta,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	created, err := s.repo.Create(ctx, order)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateIdempotency) {
			existing, getErr := s.repo.GetByIdempotency(ctx, appID, key)
			if getErr != nil {
				return dto.OrderResponse{}, response.Internal("duplicate create race", getErr)
			}
			return toOrderResponse(existing), nil
		}
		return dto.OrderResponse{}, response.Internal("failed to create order", err)
	}
	return toOrderResponse(created), nil
}

func (s *OrderService) CreateSpend(ctx context.Context, appID string, req dto.SpendOrderRequest) (dto.OrderResponse, error) {
	appID, userID, amount, key, meta, err := s.validateCreate(appID, req.UserID, req.Amount, req.IdempotencyKey, req.Metadata)
	if err != nil {
		return dto.OrderResponse{}, err
	}
	if existing, err := s.repo.GetByIdempotency(ctx, appID, key); err == nil {
		if existing.Type != model.TypeSpend || existing.UserID != userID || existing.Amount != amount {
			return dto.OrderResponse{}, response.Conflict(response.CodeKeyPayloadMismatch, "idempotency key reused with different payload")
		}
		return toOrderResponse(existing), nil
	} else if !errors.Is(err, repository.ErrOrderNotFound) {
		return dto.OrderResponse{}, response.Internal("failed to load order", err)
	}

	now := time.Now().UTC()
	orderID := uuid.New()
	order := model.Order{
		ID:             orderID,
		AppID:          appID,
		UserID:         userID,
		Amount:         amount,
		Type:           model.TypeSpend,
		Status:         model.StatusPending,
		IdempotencyKey: key,
		Metadata:       meta,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	outbox := model.OutboxEvent{
		ID:          uuid.New(),
		AggregateID: orderID,
		EventType:   model.OutboxOrderSpend,
		Payload: map[string]any{
			"app_id":          appID,
			"user_id":         userID,
			"amount":          amount,
			"reason":          model.ReasonSpend,
			"order_id":        orderID.String(),
			"idempotency_key": fmt.Sprintf("order:%s:debit", orderID.String()),
			"metadata":        meta,
		},
		CreatedAt: now,
	}
	created, err := s.repo.CreateWithOutbox(ctx, order, outbox)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateIdempotency) {
			existing, getErr := s.repo.GetByIdempotency(ctx, appID, key)
			if getErr != nil {
				return dto.OrderResponse{}, response.Internal("duplicate spend race", getErr)
			}
			return toOrderResponse(existing), nil
		}
		return dto.OrderResponse{}, response.Internal("failed to create spend order", err)
	}
	return toOrderResponse(created), nil
}

func (s *OrderService) Get(ctx context.Context, appID, orderID string) (dto.OrderResponse, error) {
	id, err := uuid.Parse(strings.TrimSpace(orderID))
	if err != nil {
		return dto.OrderResponse{}, response.BadRequest(response.CodeInvalidParams, "invalid order_id")
	}
	order, err := s.repo.GetByID(ctx, strings.TrimSpace(appID), id)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return dto.OrderResponse{}, response.NotFound(response.CodeNotFound, "order not found")
		}
		return dto.OrderResponse{}, response.Internal("failed to get order", err)
	}
	return toOrderResponse(order), nil
}

func (s *OrderService) MarkPaidFromWebhook(ctx context.Context, req dto.PaymentWebhookRequest) (dto.OrderResponse, error) {
	if strings.ToLower(strings.TrimSpace(req.Status)) != model.StatusPaid {
		return dto.OrderResponse{}, response.BadRequest(response.CodeInvalidBody, "only status=paid is supported")
	}
	orderID, err := uuid.Parse(strings.TrimSpace(req.OrderID))
	if err != nil {
		return dto.OrderResponse{}, response.BadRequest(response.CodeInvalidBody, "invalid order_id")
	}
	appID := strings.TrimSpace(req.AppID)
	order, err := s.repo.GetByID(ctx, appID, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return dto.OrderResponse{}, response.NotFound(response.CodeNotFound, "order not found")
		}
		return dto.OrderResponse{}, response.Internal("failed to load order", err)
	}
	if order.Type != model.TypePurchase {
		return dto.OrderResponse{}, response.Conflict(response.CodeAlreadyExists, "only purchase orders can be paid")
	}
	if order.Amount != req.Amount {
		return dto.OrderResponse{}, response.Conflict(response.CodeKeyPayloadMismatch, "webhook amount does not match order")
	}

	payload := map[string]any{
		"app_id":          order.AppID,
		"user_id":         order.UserID,
		"amount":          order.Amount,
		"reason":          model.ReasonTokenPurchase,
		"order_id":        order.ID.String(),
		"idempotency_key": fmt.Sprintf("order:%s:credit", order.ID.String()),
		"metadata":        order.Metadata,
	}
	outbox := model.OutboxEvent{
		ID:          uuid.New(),
		AggregateID: order.ID,
		EventType:   model.OutboxOrderPaid,
		Payload:     payload,
		CreatedAt:   time.Now().UTC(),
	}

	updated, alreadyPaid, err := s.repo.MarkPaid(ctx, appID, orderID, strings.TrimSpace(req.PaymentRef), outbox)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return dto.OrderResponse{}, response.NotFound(response.CodeNotFound, "order not found")
		}
		if errors.Is(err, repository.ErrOrderNotPending) {
			return dto.OrderResponse{}, response.Conflict(response.CodeAlreadyExists, "order is not pending")
		}
		return dto.OrderResponse{}, response.Internal("failed to mark order paid", err)
	}
	if alreadyPaid {
		s.log.Info("payment webhook idempotent replay", zap.String("order_id", orderID.String()))
	}
	return toOrderResponse(updated), nil
}

// ApplyWalletLifecycleEvent applies wallet.credited|debited|credit_failed|debit_failed.
// Non-order events (bad/non-UUID order_id, APP_FUND, etc.) are ignored (acked by caller).
func (s *OrderService) ApplyWalletLifecycleEvent(ctx context.Context, eventType, appID, orderID, reason, code, message string) error {
	reason = strings.TrimSpace(reason)
	if reason != "" && reason != model.ReasonTokenPurchase && reason != model.ReasonSpend {
		s.log.Info("wallet lifecycle event ignored (non-order reason)",
			zap.String("event", eventType),
			zap.String("reason", reason),
			zap.String("order_id", orderID),
		)
		return nil
	}

	id, err := uuid.Parse(strings.TrimSpace(orderID))
	if err != nil {
		// Poison / legacy HTTP credits (e.g. ord_abc122) — do not release/retry.
		s.log.Warn("wallet lifecycle event ignored (invalid order_id)",
			zap.String("event", eventType),
			zap.String("order_id", orderID),
			zap.Error(err),
		)
		return nil
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return fmt.Errorf("app_id required")
	}

	var expectType, fromStatus, toStatus string
	switch eventType {
	case "wallet.debited":
		expectType, fromStatus, toStatus = model.TypeSpend, model.StatusPending, model.StatusCompleted
	case "wallet.debit_failed":
		expectType, fromStatus, toStatus = model.TypeSpend, model.StatusPending, model.StatusFailed
	case "wallet.credited":
		expectType, fromStatus, toStatus = model.TypePurchase, model.StatusPaid, model.StatusCompleted
	case "wallet.credit_failed":
		expectType, fromStatus, toStatus = model.TypePurchase, model.StatusPaid, model.StatusFailed
	default:
		s.log.Warn("wallet lifecycle event ignored (unsupported event)",
			zap.String("event", eventType),
			zap.String("order_id", orderID),
		)
		return nil
	}

	order, already, err := s.repo.ApplyWalletResult(ctx, appID, id, expectType, fromStatus, toStatus, code, message)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			s.log.Warn("wallet lifecycle event for unknown order",
				zap.String("event", eventType),
				zap.String("order_id", orderID),
			)
			return nil
		}
		if errors.Is(err, repository.ErrOrderTypeMismatch) || errors.Is(err, repository.ErrOrderBadTransition) {
			s.log.Warn("wallet lifecycle event ignored",
				zap.String("event", eventType),
				zap.String("order_id", orderID),
				zap.Error(err),
			)
			return nil
		}
		return err
	}
	if already {
		s.log.Info("wallet lifecycle event idempotent",
			zap.String("event", eventType),
			zap.String("order_id", orderID),
			zap.String("status", order.Status),
		)
		return nil
	}
	s.log.Info("order status updated from wallet event",
		zap.String("event", eventType),
		zap.String("order_id", orderID),
		zap.String("status", toStatus),
	)
	return nil
}

func (s *OrderService) validateCreate(appID, userID string, amount int64, key string, meta map[string]any) (string, string, int64, string, map[string]any, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return "", "", 0, "", nil, response.Unprocessable(response.CodeInvalidRequest, "X-App-Id is required")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", "", 0, "", nil, response.BadRequest(response.CodeInvalidBody, "user_id is required")
	}
	if s.userIDPattern != nil && !s.userIDPattern.MatchString(userID) {
		return "", "", 0, "", nil, response.BadRequest(response.CodeInvalidBody, "user_id format is invalid")
	}
	if amount <= 0 {
		return "", "", 0, "", nil, response.BadRequest(response.CodeInvalidBody, "amount must be greater than 0")
	}
	if amount > s.limits.MaxOrderAmount {
		return "", "", 0, "", nil, response.Unprocessable(response.CodeLimitExceeded, "amount exceeds MAX_ORDER_AMOUNT")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", 0, "", nil, response.BadRequest(response.CodeInvalidBody, "idempotency_key is required")
	}
	return appID, userID, amount, key, meta, nil
}

func SignWebhookBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyWebhookSignature(secret, header string, body []byte) bool {
	header = strings.TrimSpace(header)
	if header == "" || secret == "" {
		return false
	}
	expected := SignWebhookBody(secret, body)
	return hmac.Equal([]byte(strings.ToLower(header)), []byte(strings.ToLower(expected)))
}

func toOrderResponse(o model.Order) dto.OrderResponse {
	out := dto.OrderResponse{
		OrderID:        o.ID.String(),
		AppID:          o.AppID,
		UserID:         o.UserID,
		Amount:         o.Amount,
		Type:           o.Type,
		Status:         o.Status,
		IdempotencyKey: o.IdempotencyKey,
		Metadata:       o.Metadata,
		CreatedAt:      o.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if o.PaymentRef != nil {
		out.PaymentRef = *o.PaymentRef
	}
	if o.FailureCode != nil {
		out.FailureCode = *o.FailureCode
	}
	if o.FailureMessage != nil {
		out.FailureMessage = *o.FailureMessage
	}
	if o.PaidAt != nil {
		out.PaidAt = o.PaidAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}
