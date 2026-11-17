package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upDropLedgerRequestID, downDropLedgerRequestID)
}

func upDropLedgerRequestID(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS request_id;
`)
	return err
}

func downDropLedgerRequestID(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries ADD COLUMN IF NOT EXISTS request_id TEXT;
`)
	return err
}
