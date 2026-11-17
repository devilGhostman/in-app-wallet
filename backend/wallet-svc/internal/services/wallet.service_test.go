package services

import (
	"context"
	"testing"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/internal/dto"
	"wallet-svc/internal/metrics"
	"wallet-svc/internal/model"
	"wallet-svc/internal/repository"
	"wallet-svc/pkg/response"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testAppID = "app-1"

type fakeRepo struct {
	wallet  model.Wallet
	entries []model.LedgerEntry
	applyFn func(repository.ApplyRequest) (repository.ApplyResult, error)
	fundFn  func(repository.FundRequest) (repository.ApplyResult, error)
}

func (f *fakeRepo) CreateWallet(_ context.Context, appID, userID string) (model.Wallet, bool, error) {
	if f.wallet.UserID == userID && f.wallet.AppID == appID {
		if f.wallet.Status == model.WalletStatusClosed {
			f.wallet.Status = model.WalletStatusActive
			return f.wallet, true, nil
		}
		return model.Wallet{}, false, repository.ErrWalletExists
	}
	f.wallet = model.Wallet{ID: uuid.New(), AppID: appID, UserID: userID, WalletType: model.WalletTypeUser, Status: model.WalletStatusActive}
	return f.wallet, true, nil
}
func (f *fakeRepo) GetWallet(_ context.Context, appID, userID string) (model.Wallet, error) {
	if f.wallet.UserID != userID || f.wallet.AppID != appID {
		return model.Wallet{}, repository.ErrWalletNotFound
	}
	return f.wallet, nil
}
func (f *fakeRepo) Apply(_ context.Context, req repository.ApplyRequest) (repository.ApplyResult, error) {
	if f.applyFn != nil {
		return f.applyFn(req)
	}
	entry := model.LedgerEntry{
		ID:            uuid.New(),
		TransactionID: uuid.New(),
		AppID:         req.AppID,
		Amount:        req.Amount,
		BalanceAfter:  req.Amount,
	}
	return repository.ApplyResult{Wallet: f.wallet, Entry: entry, TransactionID: entry.TransactionID}, nil
}
func (f *fakeRepo) FundApp(_ context.Context, req repository.FundRequest) (repository.ApplyResult, error) {
	if f.fundFn != nil {
		return f.fundFn(req)
	}
	entry := model.LedgerEntry{
		ID:            uuid.New(),
		TransactionID: uuid.New(),
		AppID:         req.AppID,
		Amount:        req.Amount,
		BalanceAfter:  req.Amount,
	}
	return repository.ApplyResult{Wallet: model.Wallet{AppID: req.AppID, Balance: req.Amount}, Entry: entry, TransactionID: entry.TransactionID}, nil
}
func (f *fakeRepo) ListLedger(context.Context, string, string, repository.LedgerListParams) (repository.LedgerPage, error) {
	return repository.LedgerPage{Items: f.entries}, nil
}
func (f *fakeRepo) ListByOrder(context.Context, string, string) ([]model.LedgerEntry, error) {
	return f.entries, nil
}
func (f *fakeRepo) SetStatus(context.Context, string, string, string, string, string, int16) (model.Wallet, error) {
	return f.wallet, nil
}
func (f *fakeRepo) ReconcileBalances(context.Context) ([]repository.ReconcileDrift, error) {
	return nil, nil
}
func (f *fakeRepo) ListUnpublishedOutbox(context.Context, int) ([]model.OutboxEvent, error) {
	return nil, nil
}
func (f *fakeRepo) MarkOutboxPublished(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func testService(repo repository.WalletRepository) *WalletService {
	return NewWalletService(repo, nil, config.LimitsConfig{
		MaxCreditAmount:  1000,
		MaxDebitAmount:   1000,
		MaxWalletBalance: 10000,
		DailyCreditCap:   5000,
	}, config.DBConfig{}, `^[a-fA-F0-9]{24}$`, &metrics.Counters{}, zap.NewNop())
}

func TestRequestHashStable(t *testing.T) {
	a := RequestHash(testAppID, "u", "CREDIT", "TOKEN_PURCHASE", 10, "ord")
	b := RequestHash(testAppID, "u", "CREDIT", "TOKEN_PURCHASE", 10, "ord")
	c := RequestHash(testAppID, "u", "CREDIT", "TOKEN_PURCHASE", 11, "ord")
	d := RequestHash("other-app", "u", "CREDIT", "TOKEN_PURCHASE", 10, "ord")
	require.Equal(t, a, b)
	require.NotEqual(t, a, c)
	require.NotEqual(t, a, d)
}

func TestCreditValidation(t *testing.T) {
	svc := testService(&fakeRepo{})
	user := "aaaaaaaaaaaaaaaaaaaaaaaa"

	_, _, err := svc.Credit(context.Background(), testAppID, "bad", "r1", dto.MutationRequest{
		Amount: 10, Reason: model.ReasonTokenPurchase, OrderID: "o1", IdempotencyKey: "order:o1:credit",
	})
	require.Error(t, err)

	_, _, err = svc.Credit(context.Background(), "", user, "r1", dto.MutationRequest{
		Amount: 10, Reason: model.ReasonTokenPurchase, OrderID: "o1", IdempotencyKey: "order:o1:credit",
	})
	require.Error(t, err)

	_, _, err = svc.Credit(context.Background(), testAppID, user, "r1", dto.MutationRequest{
		Amount: 10, Reason: model.ReasonSpend, OrderID: "o1", IdempotencyKey: "order:o1:credit",
	})
	require.Error(t, err)

	_, _, err = svc.Credit(context.Background(), testAppID, user, "r1", dto.MutationRequest{
		Amount: 10, Reason: model.ReasonTokenPurchase, OrderID: "other", IdempotencyKey: "order:o1:credit",
	})
	require.Error(t, err)

	_, _, err = svc.Credit(context.Background(), testAppID, user, "r1", dto.MutationRequest{
		Amount: 10, Reason: model.ReasonTokenPurchase, OrderID: "o1", IdempotencyKey: "order:o1:debit",
	})
	require.Error(t, err)
}

func TestMapApplyErrors(t *testing.T) {
	svc := testService(&fakeRepo{applyFn: func(repository.ApplyRequest) (repository.ApplyResult, error) {
		return repository.ApplyResult{}, repository.ErrKeyPayloadMismatch
	}})
	user := "aaaaaaaaaaaaaaaaaaaaaaaa"
	_, _, err := svc.Credit(context.Background(), testAppID, user, "r1", dto.MutationRequest{
		Amount: 10, Reason: model.ReasonTokenPurchase, OrderID: "o1", IdempotencyKey: "order:o1:credit",
	})
	var appErr *response.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeKeyPayloadMismatch, appErr.Code)
}

func TestCreateWalletRejectsUntilClosed(t *testing.T) {
	repo := &fakeRepo{}
	svc := testService(repo)
	user := "bbbbbbbbbbbbbbbbbbbbbbbb"

	first, err := svc.Create(context.Background(), testAppID, user)
	require.NoError(t, err)
	require.Equal(t, user, first.UserID)
	require.Equal(t, testAppID, first.AppID)

	_, err = svc.Create(context.Background(), testAppID, user)
	var appErr *response.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeAlreadyExists, appErr.Code)

	repo.wallet.Status = model.WalletStatusClosed
	reopened, err := svc.Create(context.Background(), testAppID, user)
	require.NoError(t, err)
	require.Equal(t, user, reopened.UserID)
	require.Equal(t, int16(model.WalletStatusActive), repo.wallet.Status)
}

func TestFundAppValidation(t *testing.T) {
	svc := testService(&fakeRepo{})
	_, _, err := svc.FundApp(context.Background(), testAppID, "r1", dto.FundAppRequest{
		Amount: 100, OrderID: "o1", IdempotencyKey: "order:o1:credit",
	})
	require.Error(t, err)

	out, status, err := svc.FundApp(context.Background(), testAppID, "r1", dto.FundAppRequest{
		Amount: 100, OrderID: "o1", IdempotencyKey: "order:o1:fund",
	})
	require.NoError(t, err)
	require.Equal(t, 201, status)
	require.Equal(t, testAppID, out.AppID)
	require.Equal(t, int64(100), out.Amount)
}
