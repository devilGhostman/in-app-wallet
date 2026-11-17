package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upInit, downInit)
}

func upInit(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    status SMALLINT NOT NULL DEFAULT 1,
    version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    entry_type TEXT NOT NULL CHECK (entry_type IN ('CREDIT', 'DEBIT')),
    reason TEXT NOT NULL CHECK (reason IN ('TOKEN_PURCHASE', 'SPEND')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    order_id TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledger_reason_type_pair CHECK (
        (reason = 'TOKEN_PURCHASE' AND entry_type = 'CREDIT')
        OR (reason = 'SPEND' AND entry_type = 'DEBIT')
    ),
    UNIQUE (wallet_id, idempotency_key),
    UNIQUE (wallet_id, reason, order_id)
);

CREATE INDEX IF NOT EXISTS ledger_entries_wallet_created_idx
    ON ledger_entries (wallet_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS ledger_entries_order_idx
    ON ledger_entries (order_id);

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

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    action TEXT NOT NULL,
    reason TEXT NOT NULL,
    request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION forbid_ledger_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
CREATE TRIGGER ledger_entries_immutable
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW
    EXECUTE FUNCTION forbid_ledger_mutation();

DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'wallet_app') THEN
        GRANT SELECT, INSERT, UPDATE ON wallets TO wallet_app;
        GRANT SELECT, INSERT ON ledger_entries TO wallet_app;
        GRANT SELECT, INSERT, UPDATE ON outbox TO wallet_app;
        GRANT SELECT, INSERT ON audit_logs TO wallet_app;
    END IF;
    IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'wallet_readonly') THEN
        GRANT SELECT ON wallets, ledger_entries, outbox, audit_logs TO wallet_readonly;
    END IF;
END $$;
`)
	return err
}

func downInit(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
DROP FUNCTION IF EXISTS forbid_ledger_mutation();
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS wallets;
`)
	return err
}
