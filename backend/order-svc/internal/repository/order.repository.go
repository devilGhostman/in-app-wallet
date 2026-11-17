package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"order-svc/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

type OrderRepository interface {
	Create(ctx context.Context, order model.Order) (model.Order, error)
	CreateWithOutbox(ctx context.Context, order model.Order, outbox model.OutboxEvent) (model.Order, error)
	GetByID(ctx context.Context, appID string, orderID uuid.UUID) (model.Order, error)
	GetByIdempotency(ctx context.Context, appID, key string) (model.Order, error)
	MarkPaid(ctx context.Context, appID string, orderID uuid.UUID, paymentRef string, outbox model.OutboxEvent) (model.Order, bool, error)
	ApplyWalletResult(ctx context.Context, appID string, orderID uuid.UUID, expectType, fromStatus, toStatus string, failureCode, failureMessage string) (model.Order, bool, error)
	ListUnpublishedOutbox(ctx context.Context, limit int) ([]model.OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error
}

type orderRepository struct {
	db *bun.DB
}

func NewOrderRepository(db *bun.DB) OrderRepository {
	return &orderRepository{db: db}
}

func (r *orderRepository) Create(ctx context.Context, order model.Order) (model.Order, error) {
	_, err := r.db.NewInsert().Model(&order).Exec(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Order{}, ErrDuplicateIdempotency
		}
		return model.Order{}, err
	}
	return order, nil
}

func (r *orderRepository) CreateWithOutbox(ctx context.Context, order model.Order, outbox model.OutboxEvent) (model.Order, error) {
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(&order).Exec(ctx); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateIdempotency
			}
			return err
		}
		if _, err := tx.NewInsert().Model(&outbox).Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return model.Order{}, err
	}
	return order, nil
}

func (r *orderRepository) GetByID(ctx context.Context, appID string, orderID uuid.UUID) (model.Order, error) {
	var order model.Order
	err := r.db.NewSelect().Model(&order).
		Where("id = ? AND app_id = ?", orderID, appID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Order{}, ErrOrderNotFound
	}
	return order, err
}

func (r *orderRepository) GetByIdempotency(ctx context.Context, appID, key string) (model.Order, error) {
	var order model.Order
	err := r.db.NewSelect().Model(&order).
		Where("app_id = ? AND idempotency_key = ?", appID, key).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Order{}, ErrOrderNotFound
	}
	return order, err
}

func (r *orderRepository) MarkPaid(
	ctx context.Context,
	appID string,
	orderID uuid.UUID,
	paymentRef string,
	outbox model.OutboxEvent,
) (model.Order, bool, error) {
	var result model.Order
	alreadyPaid := false

	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var order model.Order
		if err := tx.NewSelect().Model(&order).
			Where("id = ? AND app_id = ?", orderID, appID).
			For("UPDATE").
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOrderNotFound
			}
			return err
		}
		if order.Type != model.TypePurchase {
			return ErrOrderNotPending
		}
		if order.Status == model.StatusPaid || order.Status == model.StatusCompleted {
			alreadyPaid = true
			result = order
			return nil
		}
		if order.Status != model.StatusPending {
			return ErrOrderNotPending
		}

		now := time.Now().UTC()
		order.Status = model.StatusPaid
		order.PaymentRef = &paymentRef
		order.PaidAt = &now
		order.UpdatedAt = now

		if _, err := tx.NewUpdate().Model(&order).
			Column("status", "payment_ref", "paid_at", "updated_at").
			WherePK().
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(&outbox).Exec(ctx); err != nil {
			return err
		}
		result = order
		return nil
	})
	return result, alreadyPaid, err
}

// ApplyWalletResult moves an order from fromStatus to toStatus when type matches.
// Returns (order, alreadyTerminal, err).
func (r *orderRepository) ApplyWalletResult(
	ctx context.Context,
	appID string,
	orderID uuid.UUID,
	expectType, fromStatus, toStatus string,
	failureCode, failureMessage string,
) (model.Order, bool, error) {
	var result model.Order
	already := false

	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var order model.Order
		if err := tx.NewSelect().Model(&order).
			Where("id = ? AND app_id = ?", orderID, appID).
			For("UPDATE").
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOrderNotFound
			}
			return err
		}
		if order.Type != expectType {
			return ErrOrderTypeMismatch
		}
		if order.Status == model.StatusCompleted || order.Status == model.StatusFailed {
			already = true
			result = order
			return nil
		}
		if order.Status != fromStatus {
			return ErrOrderBadTransition
		}

		now := time.Now().UTC()
		order.Status = toStatus
		order.UpdatedAt = now
		cols := []string{"status", "updated_at"}
		if toStatus == model.StatusFailed {
			order.FailureCode = strPtr(failureCode)
			order.FailureMessage = strPtr(failureMessage)
			cols = append(cols, "failure_code", "failure_message")
		}
		if _, err := tx.NewUpdate().Model(&order).Column(cols...).WherePK().Exec(ctx); err != nil {
			return err
		}
		result = order
		return nil
	})
	return result, already, err
}

func (r *orderRepository) ListUnpublishedOutbox(ctx context.Context, limit int) ([]model.OutboxEvent, error) {
	var rows []model.OutboxEvent
	err := r.db.NewSelect().Model(&rows).
		Where("published_at IS NULL").
		OrderExpr("created_at ASC").
		Limit(int64(limit)).
		Scan(ctx)
	return rows, err
}

func (r *orderRepository) MarkOutboxPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	_, err := r.db.NewUpdate().Model((*model.OutboxEvent)(nil)).
		Set("published_at = ?", publishedAt).
		Where("id = ? AND published_at IS NULL", id).
		Exec(ctx)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
