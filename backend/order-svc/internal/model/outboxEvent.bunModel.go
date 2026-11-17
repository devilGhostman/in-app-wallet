package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	OutboxOrderPaid  = "order.paid"
	OutboxOrderSpend = "order.spend"
)

type OutboxEvent struct {
	bun.BaseModel `bun:"table:outbox,alias:ob"`

	ID          uuid.UUID      `bun:"id,pk,type:uuid"`
	AggregateID uuid.UUID      `bun:"aggregate_id,notnull,type:uuid"`
	EventType   string         `bun:"event_type,notnull"`
	Payload     map[string]any `bun:"payload,type:jsonb,notnull"`
	CreatedAt   time.Time      `bun:"created_at,notnull"`
	PublishedAt *time.Time     `bun:"published_at"`
}
