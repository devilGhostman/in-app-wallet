package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upInitOrders, downInitOrders)
}

func upInitOrders(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY,
    app_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    type TEXT NOT NULL DEFAULT 'purchase' CHECK (type IN ('purchase', 'spend')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'paid', 'completed', 'failed', 'cancelled')),
    idempotency_key TEXT NOT NULL,
    payment_ref TEXT,
    metadata JSONB,
    failure_code TEXT,
    failure_message TEXT,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS orders_app_user_created_idx
    ON orders (app_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS orders_app_status_idx
    ON orders (app_id, status);

CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS outbox_unpublished_idx
    ON outbox (created_at)
    WHERE published_at IS NULL;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'order_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO order_app;
    GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO order_app;
  END IF;
END $$;
`)
	return err
}

func downInitOrders(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS orders;
`)
	return err
}
