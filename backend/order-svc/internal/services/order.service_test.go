package services

import (
	"context"
	"testing"
	"time"

	"order-svc/internal/config"
	"order-svc/internal/dto"
	"order-svc/internal/model"
	"order-svc/internal/repository"
	"order-svc/pkg/response"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeOrderRepo struct {
	byID   map[uuid.UUID]model.Order
	byKey  map[string]model.Order
	outbox []model.OutboxEvent
}

func newFakeRepo() *fakeOrderRepo {
	return &fakeOrderRepo{
		byID:  map[uuid.UUID]model.Order{},
		byKey: map[string]model.Order{},
	}
}

func (f *fakeOrderRepo) Create(_ context.Context, order model.Order) (model.Order, error) {
	key := order.AppID + "|" + order.IdempotencyKey
	if _, ok := f.byKey[key]; ok {
		return model.Order{}, repository.ErrDuplicateIdempotency
	}
	f.byID[order.ID] = order
	f.byKey[key] = order
	return order, nil
}

func (f *fakeOrderRepo) CreateWithOutbox(_ context.Context, order model.Order, outbox model.OutboxEvent) (model.Order, error) {
	key := order.AppID + "|" + order.IdempotencyKey
	if _, ok := f.byKey[key]; ok {
		return model.Order{}, repository.ErrDuplicateIdempotency
	}
	f.byID[order.ID] = order
	f.byKey[key] = order
	f.outbox = append(f.outbox, outbox)
	return order, nil
}

func (f *fakeOrderRepo) GetByID(_ context.Context, appID string, orderID uuid.UUID) (model.Order, error) {
	o, ok := f.byID[orderID]
	if !ok || o.AppID != appID {
		return model.Order{}, repository.ErrOrderNotFound
	}
	return o, nil
}

func (f *fakeOrderRepo) GetByIdempotency(_ context.Context, appID, key string) (model.Order, error) {
	o, ok := f.byKey[appID+"|"+key]
	if !ok {
		return model.Order{}, repository.ErrOrderNotFound
	}
	return o, nil
}

func (f *fakeOrderRepo) MarkPaid(_ context.Context, appID string, orderID uuid.UUID, paymentRef string, outbox model.OutboxEvent) (model.Order, bool, error) {
	o, ok := f.byID[orderID]
	if !ok || o.AppID != appID {
		return model.Order{}, false, repository.ErrOrderNotFound
	}
	if o.Type != model.TypePurchase {
		return model.Order{}, false, repository.ErrOrderNotPending
	}
	if o.Status == model.StatusPaid || o.Status == model.StatusCompleted {
		return o, true, nil
	}
	if o.Status != model.StatusPending {
		return model.Order{}, false, repository.ErrOrderNotPending
	}
	now := time.Now().UTC()
	o.Status = model.StatusPaid
	o.PaymentRef = &paymentRef
	o.PaidAt = &now
	o.UpdatedAt = now
	f.byID[orderID] = o
	f.byKey[o.AppID+"|"+o.IdempotencyKey] = o
	f.outbox = append(f.outbox, outbox)
	return o, false, nil
}

func (f *fakeOrderRepo) ApplyWalletResult(
	_ context.Context,
	appID string,
	orderID uuid.UUID,
	expectType, fromStatus, toStatus string,
	failureCode, failureMessage string,
) (model.Order, bool, error) {
	o, ok := f.byID[orderID]
	if !ok || o.AppID != appID {
		return model.Order{}, false, repository.ErrOrderNotFound
	}
	if o.Type != expectType {
		return model.Order{}, false, repository.ErrOrderTypeMismatch
	}
	if o.Status == model.StatusCompleted || o.Status == model.StatusFailed {
		return o, true, nil
	}
	if o.Status != fromStatus {
		return model.Order{}, false, repository.ErrOrderBadTransition
	}
	o.Status = toStatus
	o.UpdatedAt = time.Now().UTC()
	if toStatus == model.StatusFailed {
		o.FailureCode = &failureCode
		o.FailureMessage = &failureMessage
	}
	f.byID[orderID] = o
	f.byKey[o.AppID+"|"+o.IdempotencyKey] = o
	return o, false, nil
}

func (f *fakeOrderRepo) ListUnpublishedOutbox(context.Context, int) ([]model.OutboxEvent, error) {
	return f.outbox, nil
}

func (f *fakeOrderRepo) MarkOutboxPublished(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func TestCreateIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, `^[a-fA-F0-9]{24}$`, zap.NewNop())
	req := dto.CreateOrderRequest{
		UserID: "aaaaaaaaaaaaaaaaaaaaaaaa", Amount: 50,
		IdempotencyKey: "create:1",
	}
	a, err := svc.Create(context.Background(), "app-1", req)
	require.NoError(t, err)
	b, err := svc.Create(context.Background(), "app-1", req)
	require.NoError(t, err)
	require.Equal(t, a.OrderID, b.OrderID)
	require.Equal(t, model.StatusPending, a.Status)
	require.Equal(t, model.TypePurchase, a.Type)
}

func TestCreateIdempotencyMismatch(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	_, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 10, IdempotencyKey: "k1",
	})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 20, IdempotencyKey: "k1",
	})
	require.Error(t, err)
	var appErr *response.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeKeyPayloadMismatch, appErr.Code)
}

func TestCreateSpendQueuesOutbox(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	out, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 10, IdempotencyKey: "spend:1",
	})
	require.NoError(t, err)
	require.Equal(t, model.TypeSpend, out.Type)
	require.Equal(t, model.StatusPending, out.Status)
	require.Len(t, repo.outbox, 1)
	require.Equal(t, model.OutboxOrderSpend, repo.outbox[0].EventType)
	require.Equal(t, model.ReasonSpend, repo.outbox[0].Payload["reason"])
	require.Equal(t, "order:"+out.OrderID+":debit", repo.outbox[0].Payload["idempotency_key"])

	again, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 10, IdempotencyKey: "spend:1",
	})
	require.NoError(t, err)
	require.Equal(t, out.OrderID, again.OrderID)
	require.Len(t, repo.outbox, 1)
}

func TestMarkPaidOnceAndIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	created, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 25, IdempotencyKey: "k2",
	})
	require.NoError(t, err)

	wh := dto.PaymentWebhookRequest{
		OrderID: created.OrderID, PaymentRef: "pay_1", Status: "paid", Amount: 25, AppID: "app-1",
	}
	paid, err := svc.MarkPaidFromWebhook(context.Background(), wh)
	require.NoError(t, err)
	require.Equal(t, model.StatusPaid, paid.Status)
	require.Len(t, repo.outbox, 1)
	require.Equal(t, "TOKEN_PURCHASE", repo.outbox[0].Payload["reason"])
	require.Equal(t, "order:"+created.OrderID+":credit", repo.outbox[0].Payload["idempotency_key"])

	paid2, err := svc.MarkPaidFromWebhook(context.Background(), wh)
	require.NoError(t, err)
	require.Equal(t, model.StatusPaid, paid2.Status)
	require.Len(t, repo.outbox, 1)
}

func TestMarkPaidRejectsSpend(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	spend, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 5, IdempotencyKey: "spend:pay",
	})
	require.NoError(t, err)
	_, err = svc.MarkPaidFromWebhook(context.Background(), dto.PaymentWebhookRequest{
		OrderID: spend.OrderID, PaymentRef: "pay_x", Status: "paid", Amount: 5, AppID: "app-1",
	})
	require.Error(t, err)
}

func TestApplyWalletLifecycleSpendAndPurchase(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())

	spend, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 7, IdempotencyKey: "spend:life",
	})
	require.NoError(t, err)
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debited", "app-1", spend.OrderID, model.ReasonSpend, "", ""))
	got, err := svc.Get(context.Background(), "app-1", spend.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusCompleted, got.Status)

	// first terminal wins
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debit_failed", "app-1", spend.OrderID, model.ReasonSpend, "X", "nope"))
	got, err = svc.Get(context.Background(), "app-1", spend.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusCompleted, got.Status)

	purchase, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 11, IdempotencyKey: "buy:life",
	})
	require.NoError(t, err)
	paid, err := svc.MarkPaidFromWebhook(context.Background(), dto.PaymentWebhookRequest{
		OrderID: purchase.OrderID, PaymentRef: "pay_2", Status: "paid", Amount: 11, AppID: "app-1",
	})
	require.NoError(t, err)
	require.Equal(t, model.StatusPaid, paid.Status)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credit_failed", "app-1", purchase.OrderID, model.ReasonTokenPurchase, "INSUFFICIENT_FUNDS", "pool empty"))
	got, err = svc.Get(context.Background(), "app-1", purchase.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusFailed, got.Status)
	require.Equal(t, "INSUFFICIENT_FUNDS", got.FailureCode)

	purchase2, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 12, IdempotencyKey: "buy:ok",
	})
	require.NoError(t, err)
	_, err = svc.MarkPaidFromWebhook(context.Background(), dto.PaymentWebhookRequest{
		OrderID: purchase2.OrderID, PaymentRef: "pay_3", Status: "paid", Amount: 12, AppID: "app-1",
	})
	require.NoError(t, err)
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", purchase2.OrderID, model.ReasonTokenPurchase, "", ""))
	got, err = svc.Get(context.Background(), "app-1", purchase2.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusCompleted, got.Status)
}

func TestApplyWalletLifecycleIgnoresWrongType(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	purchase, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 3, IdempotencyKey: "buy:wrong",
	})
	require.NoError(t, err)
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debited", "app-1", purchase.OrderID, model.ReasonSpend, "", ""))
	got, err := svc.Get(context.Background(), "app-1", purchase.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPending, got.Status)
}

func TestApplyWalletLifecycleIgnoresLegacyOrderID(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", "ord_abc122", model.ReasonTokenPurchase, "", ""))
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", "fund-1", "APP_FUND", "", ""))
}

func TestCreateSpendIdempotencyMismatch(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	_, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 10, IdempotencyKey: "spend:mismatch",
	})
	require.NoError(t, err)
	require.Len(t, repo.outbox, 1)

	_, err = svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 20, IdempotencyKey: "spend:mismatch",
	})
	require.Error(t, err)
	var appErr *response.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeKeyPayloadMismatch, appErr.Code)
	require.Len(t, repo.outbox, 1)

	_, err = svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u2", Amount: 10, IdempotencyKey: "spend:mismatch",
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeKeyPayloadMismatch, appErr.Code)
	require.Len(t, repo.outbox, 1)
}

func TestMarkPaidWebhookAmountMismatch(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	created, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 25, IdempotencyKey: "buy:amt",
	})
	require.NoError(t, err)

	_, err = svc.MarkPaidFromWebhook(context.Background(), dto.PaymentWebhookRequest{
		OrderID: created.OrderID, PaymentRef: "pay_bad", Status: "paid", Amount: 99, AppID: "app-1",
	})
	require.Error(t, err)
	var appErr *response.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeKeyPayloadMismatch, appErr.Code)
	require.Empty(t, repo.outbox)

	got, err := svc.Get(context.Background(), "app-1", created.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPending, got.Status)
}

func TestMarkPaidRejectsFailedOrder(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	created, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 15, IdempotencyKey: "buy:failed-pay",
	})
	require.NoError(t, err)

	id, err := uuid.Parse(created.OrderID)
	require.NoError(t, err)
	o := repo.byID[id]
	o.Status = model.StatusFailed
	repo.byID[id] = o
	repo.byKey[o.AppID+"|"+o.IdempotencyKey] = o

	_, err = svc.MarkPaidFromWebhook(context.Background(), dto.PaymentWebhookRequest{
		OrderID: created.OrderID, PaymentRef: "pay_f", Status: "paid", Amount: 15, AppID: "app-1",
	})
	require.Error(t, err)
	var appErr *response.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, response.CodeAlreadyExists, appErr.Code)
	require.Empty(t, repo.outbox)
}

func TestApplyWalletLifecycleDebitedReplay(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	spend, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 7, IdempotencyKey: "spend:replay",
	})
	require.NoError(t, err)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debited", "app-1", spend.OrderID, model.ReasonSpend, "", ""))
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debited", "app-1", spend.OrderID, model.ReasonSpend, "", ""))

	got, err := svc.Get(context.Background(), "app-1", spend.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusCompleted, got.Status)
}

func TestApplyWalletLifecycleCreditWhilePendingIgnored(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	purchase, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 9, IdempotencyKey: "buy:pending-credit",
	})
	require.NoError(t, err)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", purchase.OrderID, model.ReasonTokenPurchase, "", ""))
	got, err := svc.Get(context.Background(), "app-1", purchase.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPending, got.Status)
}

func TestApplyWalletLifecycleSpendDebitFailed(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	spend, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 4, IdempotencyKey: "spend:fail",
	})
	require.NoError(t, err)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debit_failed", "app-1", spend.OrderID, model.ReasonSpend, "INSUFFICIENT_FUNDS", "not enough"))
	got, err := svc.Get(context.Background(), "app-1", spend.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusFailed, got.Status)
	require.Equal(t, "INSUFFICIENT_FUNDS", got.FailureCode)
}

func TestApplyWalletLifecycleCreditedReplay(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	purchase, err := svc.Create(context.Background(), "app-1", dto.CreateOrderRequest{
		UserID: "u1", Amount: 8, IdempotencyKey: "buy:credit-replay",
	})
	require.NoError(t, err)
	_, err = svc.MarkPaidFromWebhook(context.Background(), dto.PaymentWebhookRequest{
		OrderID: purchase.OrderID, PaymentRef: "pay_r", Status: "paid", Amount: 8, AppID: "app-1",
	})
	require.NoError(t, err)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", purchase.OrderID, model.ReasonTokenPurchase, "", ""))
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", purchase.OrderID, model.ReasonTokenPurchase, "", ""))
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credit_failed", "app-1", purchase.OrderID, model.ReasonTokenPurchase, "X", "nope"))

	got, err := svc.Get(context.Background(), "app-1", purchase.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusCompleted, got.Status)
}

func TestApplyWalletLifecycleUnknownOrderUUID(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	unknown := uuid.New().String()
	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.debited", "app-1", unknown, model.ReasonSpend, "", ""))
	require.Empty(t, repo.byID)
}

func TestApplyWalletLifecycleUnsupportedEventIgnored(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	spend, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 3, IdempotencyKey: "spend:unsupported",
	})
	require.NoError(t, err)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.unknown", "app-1", spend.OrderID, model.ReasonSpend, "", ""))
	got, err := svc.Get(context.Background(), "app-1", spend.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPending, got.Status)
}

func TestApplyWalletLifecycleSpendIgnoresCredited(t *testing.T) {
	repo := newFakeRepo()
	svc := NewOrderService(repo, config.LimitsConfig{MaxOrderAmount: 1000}, "", zap.NewNop())
	spend, err := svc.CreateSpend(context.Background(), "app-1", dto.SpendOrderRequest{
		UserID: "u1", Amount: 6, IdempotencyKey: "spend:got-credit",
	})
	require.NoError(t, err)

	require.NoError(t, svc.ApplyWalletLifecycleEvent(context.Background(), "wallet.credited", "app-1", spend.OrderID, model.ReasonTokenPurchase, "", ""))
	got, err := svc.Get(context.Background(), "app-1", spend.OrderID)
	require.NoError(t, err)
	require.Equal(t, model.StatusPending, got.Status)
}

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"order_id":"x"}`)
	sig := SignWebhookBody("dev-only-payment-webhook-secret!!!!!", body)
	require.True(t, VerifyWebhookSignature("dev-only-payment-webhook-secret!!!!!", sig, body))
	require.False(t, VerifyWebhookSignature("dev-only-payment-webhook-secret!!!!!", "deadbeef", body))
}
