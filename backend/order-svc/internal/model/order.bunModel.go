package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	TypePurchase = "purchase"
	TypeSpend    = "spend"

	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"

	ReasonTokenPurchase = "TOKEN_PURCHASE"
	ReasonSpend         = "SPEND"
)

type Order struct {
	bun.BaseModel `bun:"table:orders,alias:o"`

	ID             uuid.UUID      `bun:"id,pk,type:uuid"`
	AppID          string         `bun:"app_id,notnull"`
	UserID         string         `bun:"user_id,notnull"`
	Amount         int64          `bun:"amount,notnull"`
	Type           string         `bun:"type,notnull"`
	Status         string         `bun:"status,notnull"`
	IdempotencyKey string         `bun:"idempotency_key,notnull"`
	PaymentRef     *string        `bun:"payment_ref"`
	Metadata       map[string]any `bun:"metadata,type:jsonb"`
	FailureCode    *string        `bun:"failure_code"`
	FailureMessage *string        `bun:"failure_message"`
	PaidAt         *time.Time     `bun:"paid_at"`
	CreatedAt      time.Time      `bun:"created_at,notnull"`
	UpdatedAt      time.Time      `bun:"updated_at,notnull"`
}
