package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upDoubleEntryWallets, downDoubleEntryWallets)
}

func upDoubleEntryWallets(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
-- wallets: app scoping + wallet types
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS app_id TEXT;
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS wallet_type TEXT;

UPDATE wallets
SET app_id = COALESCE(NULLIF(app_id, ''), 'legacy'),
    wallet_type = COALESCE(NULLIF(wallet_type, ''), 'USER')
WHERE app_id IS NULL OR wallet_type IS NULL;

ALTER TABLE wallets ALTER COLUMN app_id SET NOT NULL;
ALTER TABLE wallets ALTER COLUMN wallet_type SET NOT NULL;

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_user_id_key;
DROP INDEX IF EXISTS wallets_user_id_key;

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_balance_check;

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_type_check;
ALTER TABLE wallets ADD CONSTRAINT wallets_type_check CHECK (
    wallet_type IN ('USER', 'APP', 'MINT')
);

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_type_user_id_check;
ALTER TABLE wallets ADD CONSTRAINT wallets_type_user_id_check CHECK (
    (wallet_type = 'USER' AND user_id <> '' AND user_id NOT IN ('__app__', '__mint__'))
    OR (wallet_type = 'APP' AND user_id = '__app__')
    OR (wallet_type = 'MINT' AND user_id = '__mint__')
);

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_balance_nonneg_non_mint;
ALTER TABLE wallets ADD CONSTRAINT wallets_balance_nonneg_non_mint CHECK (
    wallet_type = 'MINT' OR balance >= 0
);

CREATE UNIQUE INDEX IF NOT EXISTS wallets_user_app_uidx
    ON wallets (app_id, user_id) WHERE wallet_type = 'USER';
CREATE UNIQUE INDEX IF NOT EXISTS wallets_app_uidx
    ON wallets (app_id) WHERE wallet_type = 'APP';
CREATE UNIQUE INDEX IF NOT EXISTS wallets_mint_uidx
    ON wallets (app_id) WHERE wallet_type = 'MINT';

CREATE INDEX IF NOT EXISTS wallets_app_id_idx ON wallets (app_id);

-- Ensure APP + MINT rows exist for every app_id already present
INSERT INTO wallets (id, user_id, balance, status, version, created_at, updated_at, app_id, wallet_type)
SELECT gen_random_uuid(), '__app__', 0, 1, 0, now(), now(), w.app_id, 'APP'
FROM (SELECT DISTINCT app_id FROM wallets) w
WHERE NOT EXISTS (
    SELECT 1 FROM wallets a WHERE a.app_id = w.app_id AND a.wallet_type = 'APP'
);

INSERT INTO wallets (id, user_id, balance, status, version, created_at, updated_at, app_id, wallet_type)
SELECT gen_random_uuid(), '__mint__', 0, 1, 0, now(), now(), w.app_id, 'MINT'
FROM (SELECT DISTINCT app_id FROM wallets) w
WHERE NOT EXISTS (
    SELECT 1 FROM wallets m WHERE m.app_id = w.app_id AND m.wallet_type = 'MINT'
);

-- ledger: transaction pairing + app_id
ALTER TABLE ledger_entries ADD COLUMN IF NOT EXISTS transaction_id UUID;
ALTER TABLE ledger_entries ADD COLUMN IF NOT EXISTS app_id TEXT;

ALTER TABLE ledger_entries DISABLE TRIGGER ledger_entries_immutable;

UPDATE ledger_entries le
SET app_id = w.app_id
FROM wallets w
WHERE le.wallet_id = w.id AND (le.app_id IS NULL OR le.app_id = '');

UPDATE ledger_entries
SET transaction_id = id
WHERE transaction_id IS NULL;

ALTER TABLE ledger_entries ENABLE TRIGGER ledger_entries_immutable;

ALTER TABLE ledger_entries ALTER COLUMN transaction_id SET NOT NULL;
ALTER TABLE ledger_entries ALTER COLUMN app_id SET NOT NULL;

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_reason_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_reason_check CHECK (
    reason IN ('TOKEN_PURCHASE', 'SPEND', 'APP_FUND')
);

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_reason_type_pair;

CREATE INDEX IF NOT EXISTS ledger_entries_transaction_idx ON ledger_entries (transaction_id);
CREATE INDEX IF NOT EXISTS ledger_entries_app_created_idx ON ledger_entries (app_id, created_at DESC);
`)
	return err
}

func downDoubleEntryWallets(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
DROP INDEX IF EXISTS ledger_entries_app_created_idx;
DROP INDEX IF EXISTS ledger_entries_transaction_idx;

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_reason_check;
ALTER TABLE ledger_entries ADD CONSTRAINT ledger_entries_reason_check CHECK (
    reason IN ('TOKEN_PURCHASE', 'SPEND')
);

ALTER TABLE ledger_entries DROP COLUMN IF EXISTS app_id;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS transaction_id;

DELETE FROM wallets WHERE wallet_type IN ('APP', 'MINT');

DROP INDEX IF EXISTS wallets_mint_uidx;
DROP INDEX IF EXISTS wallets_app_uidx;
DROP INDEX IF EXISTS wallets_user_app_uidx;
DROP INDEX IF EXISTS wallets_app_id_idx;

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_balance_nonneg_non_mint;
ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_type_user_id_check;
ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_type_check;

ALTER TABLE wallets DROP COLUMN IF EXISTS wallet_type;
ALTER TABLE wallets DROP COLUMN IF EXISTS app_id;

ALTER TABLE wallets ADD CONSTRAINT wallets_balance_check CHECK (balance >= 0);
ALTER TABLE wallets ADD CONSTRAINT wallets_user_id_key UNIQUE (user_id);
`)
	return err
}
