package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upFixLedgerBalanceBefore, downFixLedgerBalanceBefore)
}

// upFixLedgerBalanceBefore repairs a prior failed apply of balance_before
// (append-only trigger blocked the backfill while the migration was marked applied).
func upFixLedgerBalanceBefore(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE ledger_entries
    ADD COLUMN IF NOT EXISTS balance_before BIGINT;

ALTER TABLE ledger_entries DISABLE TRIGGER ledger_entries_immutable;

UPDATE ledger_entries
SET balance_before = CASE
    WHEN entry_type = 'CREDIT' THEN balance_after - amount
    WHEN entry_type = 'DEBIT' THEN balance_after + amount
    ELSE balance_after
END
WHERE balance_before IS NULL;

ALTER TABLE ledger_entries ENABLE TRIGGER ledger_entries_immutable;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'ledger_entries'
          AND column_name = 'balance_before'
          AND is_nullable = 'YES'
    ) THEN
        ALTER TABLE ledger_entries ALTER COLUMN balance_before SET NOT NULL;
    END IF;
END $$;
`)
	return err
}

func downFixLedgerBalanceBefore(ctx context.Context, db *bun.DB) error {
	// Keep column; original down lives on 20261103160000.
	return nil
}
