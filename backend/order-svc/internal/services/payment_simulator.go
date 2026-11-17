package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// PaymentSimulator is a gimmick gateway that POSTs a signed webhook like a real PSP.
type PaymentSimulator struct {
	repo   repository.OrderRepository
	cfg    config.PaymentConfig
	client *http.Client
	log    *zap.Logger
}

func NewPaymentSimulator(repo repository.OrderRepository, cfg config.PaymentConfig, log *zap.Logger) *PaymentSimulator {
	return &PaymentSimulator{
		repo: repo,
		cfg:  cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: log,
	}
}

func (s *PaymentSimulator) Simulate(ctx context.Context, appID string, req dto.SimulatePaymentRequest) (dto.SimulatePaymentResponse, error) {
	orderID, err := uuid.Parse(strings.TrimSpace(req.OrderID))
	if err != nil {
		return dto.SimulatePaymentResponse{}, response.BadRequest(response.CodeInvalidBody, "invalid order_id")
	}
	order, err := s.repo.GetByID(ctx, strings.TrimSpace(appID), orderID)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return dto.SimulatePaymentResponse{}, response.NotFound(response.CodeNotFound, "order not found")
		}
		return dto.SimulatePaymentResponse{}, response.Internal("failed to load order", err)
	}
	if order.Status != model.StatusPending {
		return dto.SimulatePaymentResponse{}, response.Conflict(response.CodeAlreadyExists, "order is not pending")
	}

	paymentRef := "pay_" + uuid.NewString()
	payload := dto.PaymentWebhookRequest{
		OrderID:    order.ID.String(),
		PaymentRef: paymentRef,
		Status:     model.StatusPaid,
		Amount:     order.Amount,
		AppID:      order.AppID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return dto.SimulatePaymentResponse{}, response.Internal("failed to encode webhook body", err)
	}
	sig := SignWebhookBody(s.cfg.WebhookSecret, body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return dto.SimulatePaymentResponse{}, response.Internal("failed to build webhook request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Payment-Signature", sig)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		s.log.Error("fake gateway webhook call failed", zap.Error(err), zap.String("url", s.cfg.WebhookURL))
		return dto.SimulatePaymentResponse{}, response.ServiceUnavailable(response.CodeServiceUnavailable, "webhook call failed", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.log.Error("fake gateway webhook rejected",
			zap.Int("status", resp.StatusCode),
			zap.ByteString("body", respBody),
		)
		return dto.SimulatePaymentResponse{}, response.ServiceUnavailable(
			response.CodeServiceUnavailable,
			fmt.Sprintf("webhook returned %d", resp.StatusCode),
		)
	}

	return dto.SimulatePaymentResponse{
		OrderID:    order.ID.String(),
		PaymentRef: paymentRef,
		Status:     model.StatusPaid,
		WebhookOK:  true,
	}, nil
}
