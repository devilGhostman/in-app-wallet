package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(upAppWalletShards, downAppWalletShards)
}

func upAppWalletShards(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS shard_id SMALLINT NOT NULL DEFAULT 0;

-- Replace single-APP uniqueness with per-shard uniqueness.
DROP INDEX IF EXISTS wallets_app_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS wallets_app_shard_uidx
    ON wallets (app_id, shard_id) WHERE wallet_type = 'APP';

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_shard_id_check;
ALTER TABLE wallets ADD CONSTRAINT wallets_shard_id_check CHECK (
    shard_id >= 0
    AND (
        (wallet_type IN ('USER', 'MINT') AND shard_id = 0)
        OR wallet_type = 'APP'
    )
);
`)
	return err
}

func downAppWalletShards(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `
-- Keep only shard 0 APP wallets so the old unique index can apply.
DELETE FROM wallets WHERE wallet_type = 'APP' AND shard_id <> 0;

ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_shard_id_check;
DROP INDEX IF EXISTS wallets_app_shard_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS wallets_app_uidx
    ON wallets (app_id) WHERE wallet_type = 'APP';
ALTER TABLE wallets DROP COLUMN IF EXISTS shard_id;
`)
	return err
}
