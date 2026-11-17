package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upSpendAndResults, downSpendAndResults)
}

func upSpendAndResults(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE orders ADD COLUMN IF NOT EXISTS type TEXT;
UPDATE orders SET type = 'purchase' WHERE type IS NULL;
ALTER TABLE orders ALTER COLUMN type SET NOT NULL;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_type_check;
ALTER TABLE orders ADD CONSTRAINT orders_type_check CHECK (type IN ('purchase', 'spend'));

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check
  CHECK (status IN ('pending', 'paid', 'completed', 'failed', 'cancelled'));

ALTER TABLE orders ADD COLUMN IF NOT EXISTS failure_code TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS failure_message TEXT;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'order_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO order_app;
  END IF;
END $$;
`)
	return err
}

func downSpendAndResults(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE orders DROP COLUMN IF EXISTS failure_message;
ALTER TABLE orders DROP COLUMN IF EXISTS failure_code;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_type_check;
ALTER TABLE orders DROP COLUMN IF EXISTS type;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check
  CHECK (status IN ('pending', 'paid', 'failed', 'cancelled'));
`)
	return err
}
