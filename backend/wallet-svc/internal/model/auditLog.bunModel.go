package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type AuditLog struct {
	bun.BaseModel `bun:"table:audit_logs,alias:al"`

	ID        uuid.UUID `bun:"id,pk,type:uuid"`
	UserID    string    `bun:"user_id,notnull"`
	Action    string    `bun:"action,notnull"`
	Reason    string    `bun:"reason,notnull"`
	RequestID string    `bun:"request_id"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}
