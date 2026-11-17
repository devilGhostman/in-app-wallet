package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upAddLedgerBalanceBefore, downAddLedgerBalanceBefore)
}

func upAddLedgerBalanceBefore(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries
    ADD COLUMN IF NOT EXISTS balance_before BIGINT;

-- Append-only trigger blocks UPDATEs; disable only for this backfill.
ALTER TABLE ledger_entries DISABLE TRIGGER ledger_entries_immutable;

UPDATE ledger_entries
SET balance_before = CASE
    WHEN entry_type = 'CREDIT' THEN balance_after - amount
    WHEN entry_type = 'DEBIT' THEN balance_after + amount
    ELSE balance_after
END
WHERE balance_before IS NULL;

ALTER TABLE ledger_entries ENABLE TRIGGER ledger_entries_immutable;

ALTER TABLE ledger_entries
    ALTER COLUMN balance_before SET NOT NULL;
`)
	return err
}

func downAddLedgerBalanceBefore(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS balance_before;
`)
	return err
}
