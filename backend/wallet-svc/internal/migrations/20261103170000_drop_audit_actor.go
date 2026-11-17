package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upDropAuditActor, downDropAuditActor)
}

func upDropAuditActor(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor;
`)
	return err
}

func downDropAuditActor(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor TEXT NOT NULL DEFAULT '';
`)
	return err
}
