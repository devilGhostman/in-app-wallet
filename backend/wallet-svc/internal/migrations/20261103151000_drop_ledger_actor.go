package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upDropLedgerActor, downDropLedgerActor)
}

func upDropLedgerActor(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS actor;
`)
	return err
}

func downDropLedgerActor(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries ADD COLUMN IF NOT EXISTS actor TEXT NOT NULL DEFAULT '';
`)
	return err
}
