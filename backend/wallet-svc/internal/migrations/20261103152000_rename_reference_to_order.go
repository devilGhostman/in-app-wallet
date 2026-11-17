package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upRenameReferenceToOrder, downRenameReferenceToOrder)
}

func upRenameReferenceToOrder(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'ledger_entries' AND column_name = 'reference_id'
    ) THEN
        ALTER TABLE ledger_entries RENAME COLUMN reference_id TO order_id;
    END IF;
END $$;

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_wallet_id_reason_reference_id_key;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_wallet_id_reason_order_id_key;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_wallet_id_reason_order_id_key UNIQUE (wallet_id, reason, order_id);

DROP INDEX IF EXISTS ledger_entries_reference_idx;
CREATE INDEX IF NOT EXISTS ledger_entries_order_idx ON ledger_entries (order_id);
`)
	return err
}

func downRenameReferenceToOrder(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'ledger_entries' AND column_name = 'order_id'
    ) THEN
        ALTER TABLE ledger_entries RENAME COLUMN order_id TO reference_id;
    END IF;
END $$;

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_wallet_id_reason_order_id_key;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_wallet_id_reason_reference_id_key;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_wallet_id_reason_reference_id_key UNIQUE (wallet_id, reason, reference_id);

DROP INDEX IF EXISTS ledger_entries_order_idx;
CREATE INDEX IF NOT EXISTS ledger_entries_reference_idx ON ledger_entries (reference_id);
`)
	return err
}
