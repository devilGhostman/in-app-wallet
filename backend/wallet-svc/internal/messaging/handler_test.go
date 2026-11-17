package messaging

import (
	"context"
	"testing"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/internal/dto"
	"wallet-svc/internal/metrics"
	"wallet-svc/internal/model"
	"wallet-svc/internal/repository"
	"wallet-svc/internal/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeRepo struct {
	lastApply repository.ApplyRequest
}

func (f *fakeRepo) CreateWallet(context.Context, string, string) (model.Wallet, bool, error) {
	return model.Wallet{}, false, nil
}
func (f *fakeRepo) GetWallet(context.Context, string, string) (model.Wallet, error) {
	return model.Wallet{}, nil
}
func (f *fakeRepo) Apply(_ context.Context, req repository.ApplyRequest) (repository.ApplyResult, error) {
	f.lastApply = req
	id := uuid.New()
	return repository.ApplyResult{
		Wallet:        model.Wallet{AppID: req.AppID, UserID: req.UserID, Balance: req.Amount},
		Entry:         model.LedgerEntry{ID: id, Amount: req.Amount, BalanceAfter: req.Amount},
		TransactionID: id,
	}, nil
}
func (f *fakeRepo) FundApp(context.Context, repository.FundRequest) (repository.ApplyResult, error) {
	return repository.ApplyResult{}, nil
}
func (f *fakeRepo) ListLedger(context.Context, string, string, repository.LedgerListParams) (repository.LedgerPage, error) {
	return repository.LedgerPage{}, nil
}
func (f *fakeRepo) ListByOrder(context.Context, string, string) ([]model.LedgerEntry, error) {
	return nil, nil
}
func (f *fakeRepo) SetStatus(context.Context, string, string, string, string, string, int16) (model.Wallet, error) {
	return model.Wallet{}, nil
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

func TestWalletMutationHandlerCredit(t *testing.T) {
	repo := &fakeRepo{}
	svc := services.NewWalletService(repo, nil, config.LimitsConfig{
		MaxCreditAmount: 1000, MaxDebitAmount: 1000, MaxWalletBalance: 10000, DailyCreditCap: 5000,
	}, config.DBConfig{}, `^[a-fA-F0-9]{24}$`, &metrics.Counters{}, zap.NewNop())
	h := NewWalletMutationHandler(svc)

	cmd := WalletMutationCommand{
		AppID: "app-1", UserID: "aaaaaaaaaaaaaaaaaaaaaaaa", Amount: 25,
		Reason: model.ReasonTokenPurchase, OrderID: "o1", IdempotencyKey: "order:o1:credit",
	}
	out, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.Equal(t, int64(25), out.Amount)
	require.Equal(t, "app-1", repo.lastApply.AppID)
	require.Equal(t, model.EntryCredit, repo.lastApply.EntryType)
	_ = dto.MutationResponse{}
}
