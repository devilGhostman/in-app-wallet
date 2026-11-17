package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/internal/dto"
	"wallet-svc/internal/metrics"
	"wallet-svc/internal/model"
	"wallet-svc/internal/repository"
	"wallet-svc/pkg/response"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	defaultUserIDPattern   = regexp.MustCompile(`^[a-fA-F0-9]{24}$`)
	idempotencyPattern     = regexp.MustCompile(`^order:([^:]+):(credit|debit)$`)
	fundIdempotencyPattern = regexp.MustCompile(`^order:([^:]+):fund$`)
)

type WalletService struct {
	repo     repository.WalletRepository
	cache    repository.BalanceCache
	limits   config.LimitsConfig
	dbCfg    config.DBConfig
	userIDRe *regexp.Regexp
	metrics  *metrics.Counters
	logger   *zap.Logger
}

func NewWalletService(
	repo repository.WalletRepository,
	cache repository.BalanceCache,
	limits config.LimitsConfig,
	dbCfg config.DBConfig,
	userIDPattern string,
	counters *metrics.Counters,
	logger *zap.Logger,
) *WalletService {
	re := defaultUserIDPattern
	if userIDPattern != "" {
		if compiled, err := regexp.Compile(userIDPattern); err == nil {
			re = compiled
		}
	}
	return &WalletService{
		repo:     repo,
		cache:    cache,
		limits:   limits,
		dbCfg:    dbCfg,
		userIDRe: re,
		metrics:  counters,
		logger:   logger,
	}
}

func (s *WalletService) Create(ctx context.Context, appID, userID string) (dto.WalletResponse, error) {
	if err := validateAppID(appID); err != nil {
		return dto.WalletResponse{}, err
	}
	if err := s.validateUserID(userID); err != nil {
		return dto.WalletResponse{}, err
	}
	w, _, err := s.repo.CreateWallet(ctx, appID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrWalletExists) {
			return dto.WalletResponse{}, response.Conflict(response.CodeAlreadyExists, "Wallet already exists")
		}
		return dto.WalletResponse{}, response.Internal("Failed to create wallet", err)
	}
	return toWalletDTO(w), nil
}

func (s *WalletService) Get(ctx context.Context, appID, userID string) (dto.WalletResponse, error) {
	if err := validateAppID(appID); err != nil {
		return dto.WalletResponse{}, err
	}
	if err := s.validateUserID(userID); err != nil {
		return dto.WalletResponse{}, err
	}
	w, err := s.repo.GetWallet(ctx, appID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrWalletNotFound) {
			return dto.WalletResponse{}, response.NotFound(response.CodeNotFound, "Wallet not found")
		}
		return dto.WalletResponse{}, response.Internal("Failed to load wallet", err)
	}
	return toWalletDTO(w), nil
}

func (s *WalletService) Credit(ctx context.Context, appID, userID, requestID string, req dto.MutationRequest) (dto.MutationResponse, int, error) {
	return s.apply(ctx, appID, userID, requestID, req, model.EntryCredit, model.ReasonTokenPurchase)
}

func (s *WalletService) Debit(ctx context.Context, appID, userID, requestID string, req dto.MutationRequest) (dto.MutationResponse, int, error) {
	return s.apply(ctx, appID, userID, requestID, req, model.EntryDebit, model.ReasonSpend)
}

func (s *WalletService) FundApp(ctx context.Context, appID, requestID string, req dto.FundAppRequest) (dto.MutationResponse, int, error) {
	if err := validateAppID(appID); err != nil {
		return dto.MutationResponse{}, 0, err
	}
	if req.Amount <= 0 {
		return dto.MutationResponse{}, 0, response.Unprocessable(response.CodeInvalidRequest, "amount must be greater than 0")
	}
	matches := fundIdempotencyPattern.FindStringSubmatch(req.IdempotencyKey)
	if matches == nil {
		return dto.MutationResponse{}, 0, response.Unprocessable(response.CodeInvalidRequest, "idempotency_key must be order:{orderId}:fund")
	}
	if req.OrderID != matches[1] {
		return dto.MutationResponse{}, 0, response.Unprocessable(response.CodeInvalidRequest, "order_id must match order id in idempotency_key")
	}

	hash := RequestHash(appID, "", model.EntryCredit, model.ReasonAppFund, req.Amount, req.OrderID)
	fundReq := repository.FundRequest{
		AppID:          appID,
		Amount:         req.Amount,
		IdempotencyKey: req.IdempotencyKey,
		OrderID:        req.OrderID,
		RequestHash:    hash,
		Metadata:       req.Metadata,
		LockTimeout:    s.dbCfg.LockTimeout,
		StmtTimeout:    s.dbCfg.StatementTimeout,
	}

	var result repository.ApplyResult
	err := retryOnLock(ctx, func() error {
		var applyErr error
		result, applyErr = s.repo.FundApp(ctx, fundReq)
		return applyErr
	})
	if err != nil {
		return dto.MutationResponse{}, 0, s.mapApplyError(err, model.EntryCredit)
	}

	if result.Duplicate {
		s.metrics.Replays.Add(1)
	} else {
		s.metrics.Credits.Add(1)
	}

	s.logger.Info("app fund",
		zap.String("app_id", appID),
		zap.String("idempotency_key", req.IdempotencyKey),
		zap.String("request_id", requestID),
		zap.Bool("duplicate", result.Duplicate),
		zap.Int64("amount", req.Amount),
		zap.Int64("app_pool_balance", result.Wallet.Balance),
	)

	status := 201
	if result.Duplicate {
		status = 200
	}
	return dto.MutationResponse{
		AppID:         appID,
		Amount:        result.Entry.Amount,
		Balance:       result.Wallet.Balance,
		LedgerID:      result.Entry.ID.String(),
		TransactionID: result.TransactionID.String(),
		Duplicate:     result.Duplicate,
	}, status, nil
}

func (s *WalletService) apply(
	ctx context.Context,
	appID, userID, requestID string,
	req dto.MutationRequest,
	entryType, expectedReason string,
) (dto.MutationResponse, int, error) {
	if err := validateAppID(appID); err != nil {
		return dto.MutationResponse{}, 0, err
	}
	if err := s.validateUserID(userID); err != nil {
		return dto.MutationResponse{}, 0, err
	}
	if err := validateMutation(req, entryType, expectedReason); err != nil {
		return dto.MutationResponse{}, 0, err
	}
	if req.Amount <= 0 {
		return dto.MutationResponse{}, 0, response.Unprocessable(response.CodeInvalidRequest, "amount must be greater than 0")
	}

	hash := RequestHash(appID, userID, entryType, req.Reason, req.Amount, req.OrderID)
	applyReq := repository.ApplyRequest{
		AppID:          appID,
		UserID:         userID,
		EntryType:      entryType,
		Reason:         req.Reason,
		Amount:         req.Amount,
		IdempotencyKey: req.IdempotencyKey,
		OrderID:        req.OrderID,
		RequestHash:    hash,
		Metadata:       sanitizeMetadata(req.Metadata, req.Reason),
		Limits:         s.limits,
		LockTimeout:    s.dbCfg.LockTimeout,
		StmtTimeout:    s.dbCfg.StatementTimeout,
	}

	var result repository.ApplyResult
	err := retryOnLock(ctx, func() error {
		var applyErr error
		result, applyErr = s.repo.Apply(ctx, applyReq)
		return applyErr
	})
	if err != nil {
		return dto.MutationResponse{}, 0, s.mapApplyError(err, entryType)
	}

	if result.Duplicate {
		s.metrics.Replays.Add(1)
	} else if entryType == model.EntryCredit {
		s.metrics.Credits.Add(1)
	} else {
		s.metrics.Debits.Add(1)
	}

	s.maybeCache(ctx, appID, userID, result.Wallet.Balance, result.Wallet.Version)

	s.logger.Info("wallet mutation",
		zap.String("app_id", appID),
		zap.String("user_id", userID),
		zap.String("entry_type", entryType),
		zap.String("idempotency_key", req.IdempotencyKey),
		zap.String("request_id", requestID),
		zap.String("transaction_id", result.TransactionID.String()),
		zap.Bool("duplicate", result.Duplicate),
		zap.Int64("amount", req.Amount),
		zap.Int64("balance", result.Entry.BalanceAfter),
	)

	status := 201
	if result.Duplicate {
		status = 200
	}
	return dto.MutationResponse{
		AppID:         appID,
		UserID:        userID,
		Amount:        result.Entry.Amount,
		Balance:       result.Entry.BalanceAfter,
		LedgerID:      result.Entry.ID.String(),
		TransactionID: result.TransactionID.String(),
		Duplicate:     result.Duplicate,
	}, status, nil
}

func (s *WalletService) Ledger(ctx context.Context, appID, userID string, q dto.LedgerQuery) (dto.LedgerPageResponse, error) {
	if err := validateAppID(appID); err != nil {
		return dto.LedgerPageResponse{}, err
	}
	if err := s.validateUserID(userID); err != nil {
		return dto.LedgerPageResponse{}, err
	}
	params, err := buildLedgerListParams(q)
	if err != nil {
		return dto.LedgerPageResponse{}, err
	}
	page, err := s.repo.ListLedger(ctx, appID, userID, params)
	if err != nil {
		if errors.Is(err, repository.ErrWalletNotFound) {
			return dto.LedgerPageResponse{}, response.NotFound(response.CodeNotFound, "Wallet not found")
		}
		s.logger.Error("list ledger failed", zap.Error(err), zap.String("user_id", userID))
		return dto.LedgerPageResponse{}, response.Internal("Failed to list ledger", err)
	}
	items := make([]dto.LedgerItem, 0, len(page.Items))
	for _, e := range page.Items {
		items = append(items, toLedgerItem(e))
	}
	return dto.LedgerPageResponse{Transactions: items, NextCursor: page.NextCursor}, nil
}

func (s *WalletService) ByOrder(ctx context.Context, appID, orderID string) ([]dto.LedgerItem, error) {
	if err := validateAppID(appID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(orderID) == "" {
		return nil, response.Unprocessable(response.CodeInvalidRequest, "order_id is required")
	}
	items, err := s.repo.ListByOrder(ctx, appID, orderID)
	if err != nil {
		return nil, response.Internal("Failed to lookup order", err)
	}
	out := make([]dto.LedgerItem, 0, len(items))
	for _, e := range items {
		out = append(out, toLedgerItem(e))
	}
	return out, nil
}

func (s *WalletService) Freeze(ctx context.Context, appID, userID, reason, requestID string) (dto.WalletResponse, error) {
	return s.setStatus(ctx, appID, userID, "freeze", reason, requestID, model.WalletStatusFrozen)
}

func (s *WalletService) Unfreeze(ctx context.Context, appID, userID, reason, requestID string) (dto.WalletResponse, error) {
	return s.setStatus(ctx, appID, userID, "unfreeze", reason, requestID, model.WalletStatusActive)
}

func (s *WalletService) Close(ctx context.Context, appID, userID, reason, requestID string) (dto.WalletResponse, error) {
	return s.setStatus(ctx, appID, userID, "close", reason, requestID, model.WalletStatusClosed)
}

func (s *WalletService) setStatus(ctx context.Context, appID, userID, action, reason, requestID string, status int16) (dto.WalletResponse, error) {
	if err := validateAppID(appID); err != nil {
		return dto.WalletResponse{}, err
	}
	if err := s.validateUserID(userID); err != nil {
		return dto.WalletResponse{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return dto.WalletResponse{}, response.Unprocessable(response.CodeInvalidRequest, "reason is required")
	}
	w, err := s.repo.SetStatus(ctx, appID, userID, action, reason, requestID, status)
	if err != nil {
		if errors.Is(err, repository.ErrWalletNotFound) {
			return dto.WalletResponse{}, response.NotFound(response.CodeNotFound, "Wallet not found")
		}
		if errors.Is(err, repository.ErrWalletClosed) {
			return dto.WalletResponse{}, response.Unprocessable(response.CodeWalletClosed, "Wallet is closed")
		}
		return dto.WalletResponse{}, response.Internal("Failed to update wallet status", err)
	}
	s.logger.Info("wallet status change",
		zap.String("app_id", appID),
		zap.String("user_id", userID),
		zap.String("action", action),
		zap.String("reason", reason),
		zap.String("request_id", requestID),
	)
	return toWalletDTO(w), nil
}

func (s *WalletService) Reconcile(ctx context.Context) (dto.ReconcileResponse, error) {
	drifts, err := s.repo.ReconcileBalances(ctx)
	if err != nil {
		return dto.ReconcileResponse{}, response.Internal("Failed to reconcile", err)
	}
	if len(drifts) > 0 {
		s.logger.Error("wallet balance drift detected", zap.Int("count", len(drifts)))
	}
	return dto.ReconcileResponse{DriftCount: len(drifts), Drifts: drifts}, nil
}

func (s *WalletService) mapApplyError(err error, entryType string) error {
	switch {
	case errors.Is(err, repository.ErrKeyPayloadMismatch):
		s.metrics.KeyPayloadMismatch.Add(1)
		s.logger.Warn("key payload mismatch")
		return response.Conflict(response.CodeKeyPayloadMismatch, "Same key or order already used with a different payload")
	case errors.Is(err, repository.ErrInsufficientFunds):
		s.metrics.InsufficientFunds.Add(1)
		return response.Unprocessable(response.CodeInsufficientFunds, "Insufficient funds")
	case errors.Is(err, repository.ErrWalletFrozen):
		return response.Unprocessable(response.CodeWalletFrozen, "Wallet is frozen")
	case errors.Is(err, repository.ErrWalletClosed):
		return response.Unprocessable(response.CodeWalletClosed, "Wallet is closed")
	case errors.Is(err, repository.ErrLimitExceeded):
		s.metrics.LimitExceeded.Add(1)
		return response.Unprocessable(response.CodeLimitExceeded, "Amount, balance or daily cap exceeded")
	case errors.Is(err, repository.ErrLockTimeout):
		s.metrics.LockTimeouts.Add(1)
		return response.ServiceUnavailable(response.CodeLockTimeout, "Wallet is busy; retry with the same key", err)
	default:
		s.metrics.LedgerWriteErrors.Add(1)
		s.logger.Error("ledger write failed", zap.Error(err), zap.String("entry_type", entryType))
		return response.Internal("Failed to apply wallet mutation", err)
	}
}

func (s *WalletService) validateUserID(userID string) error {
	if userID == "" || !s.userIDRe.MatchString(userID) {
		return response.Unprocessable(response.CodeInvalidRequest, "invalid user_id")
	}
	return nil
}

func validateAppID(appID string) error {
	if strings.TrimSpace(appID) == "" {
		return response.Unprocessable(response.CodeInvalidRequest, "X-App-Id header is required")
	}
	return nil
}

func (s *WalletService) maybeCache(ctx context.Context, appID, userID string, balance, version int64) {
	if s.cache == nil {
		return
	}
	if err := s.cache.SetIfNewer(ctx, appID, userID, balance, version); err != nil {
		s.logger.Warn("balance cache write failed", zap.Error(err), zap.String("user_id", userID))
	}
}

func buildLedgerListParams(q dto.LedgerQuery) (repository.LedgerListParams, error) {
	params := repository.LedgerListParams{
		Limit:     q.Limit,
		EntryType: strings.TrimSpace(q.EntryType),
		Reason:    strings.TrimSpace(q.Reason),
	}
	if params.EntryType != "" && params.EntryType != model.EntryCredit && params.EntryType != model.EntryDebit {
		return repository.LedgerListParams{}, response.Unprocessable(response.CodeInvalidRequest, "entry_type must be CREDIT or DEBIT")
	}
	if params.Reason != "" &&
		params.Reason != model.ReasonTokenPurchase &&
		params.Reason != model.ReasonSpend &&
		params.Reason != model.ReasonAppFund {
		return repository.LedgerListParams{}, response.Unprocessable(response.CodeInvalidRequest, "reason must be TOKEN_PURCHASE, SPEND, or APP_FUND")
	}
	switch strings.ToLower(strings.TrimSpace(q.Sort)) {
	case "", "desc":
		params.SortAsc = false
	case "asc":
		params.SortAsc = true
	default:
		return repository.LedgerListParams{}, response.Unprocessable(response.CodeInvalidRequest, "sort must be asc or desc")
	}
	if q.Cursor != "" {
		parts := strings.SplitN(q.Cursor, "|", 2)
		if len(parts) != 2 {
			return repository.LedgerListParams{}, response.Unprocessable(response.CodeInvalidRequest, "invalid cursor")
		}
		t, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return repository.LedgerListParams{}, response.Unprocessable(response.CodeInvalidRequest, "invalid cursor")
		}
		id, err := uuid.Parse(parts[1])
		if err != nil {
			return repository.LedgerListParams{}, response.Unprocessable(response.CodeInvalidRequest, "invalid cursor")
		}
		params.CursorTime, params.CursorID = t, id
	}
	return params, nil
}

func validateMutation(req dto.MutationRequest, entryType, expectedReason string) error {
	if req.Reason != expectedReason {
		return response.Unprocessable(response.CodeInvalidRequest, "reason does not match operation")
	}
	matches := idempotencyPattern.FindStringSubmatch(req.IdempotencyKey)
	if matches == nil {
		return response.Unprocessable(response.CodeInvalidRequest, "idempotency_key must be order:{orderId}:credit|debit")
	}
	orderID, suffix := matches[1], matches[2]
	if req.OrderID != orderID {
		return response.Unprocessable(response.CodeInvalidRequest, "order_id must match order id in idempotency_key")
	}
	wantSuffix := "credit"
	if entryType == model.EntryDebit {
		wantSuffix = "debit"
	}
	if suffix != wantSuffix {
		return response.Unprocessable(response.CodeInvalidRequest, "idempotency_key suffix does not match operation")
	}
	return nil
}

func sanitizeMetadata(meta map[string]any, reason string) map[string]any {
	if reason != model.ReasonTokenPurchase || len(meta) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"psp_payment_id": {},
		"pack_id":        {},
		"price_version":  {},
		"fiat_amount":    {},
		"currency":       {},
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		if _, ok := allowed[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func RequestHash(appID, userID, entryType, reason string, amount int64, orderID string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%s|%s|%d|%s", appID, userID, entryType, reason, amount, orderID))
	return hex.EncodeToString(sum[:])
}

func retryOnLock(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil || !errors.Is(err, repository.ErrLockTimeout) {
			return err
		}
		delay := time.Duration(20+rand.Intn(40)) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func toWalletDTO(w model.Wallet) dto.WalletResponse {
	return dto.WalletResponse{
		AppID:   w.AppID,
		UserID:  w.UserID,
		Balance: w.Balance,
		Status:  w.Status,
		Version: w.Version,
	}
}

func toLedgerItem(e model.LedgerEntry) dto.LedgerItem {
	return dto.LedgerItem{
		LedgerID:       e.ID.String(),
		TransactionID:  e.TransactionID.String(),
		AppID:          e.AppID,
		EntryType:      e.EntryType,
		Reason:         e.Reason,
		Amount:         e.Amount,
		BalanceBefore:  e.BalanceBefore,
		BalanceAfter:   e.BalanceAfter,
		OrderID:        e.OrderID,
		IdempotencyKey: e.IdempotencyKey,
		CreatedAt:      e.CreatedAt.UTC().Format(time.RFC3339Nano),
		Metadata:       e.Metadata,
	}
}
