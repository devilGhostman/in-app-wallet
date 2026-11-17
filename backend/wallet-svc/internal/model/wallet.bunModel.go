package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	WalletStatusActive int16 = 1
	WalletStatusFrozen int16 = 0
	WalletStatusClosed int16 = 2

	WalletTypeUser = "USER"
	WalletTypeApp  = "APP"
	WalletTypeMint = "MINT"

	AppWalletUserID  = "__app__"
	MintWalletUserID = "__mint__"

	DefaultAppWalletShards = 16
)

type Wallet struct {
	bun.BaseModel `bun:"table:wallets,alias:w"`

	ID         uuid.UUID `bun:"id,pk,type:uuid"`
	AppID      string    `bun:"app_id,notnull"`
	UserID     string    `bun:"user_id,notnull"`
	WalletType string    `bun:"wallet_type,notnull"`
	ShardID    int16     `bun:"shard_id,notnull"`
	Balance    int64     `bun:"balance,notnull"`
	Status     int16     `bun:"status,notnull"`
	Version    int64     `bun:"version,notnull"`
	CreatedAt  time.Time `bun:"created_at,notnull"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"`
}
