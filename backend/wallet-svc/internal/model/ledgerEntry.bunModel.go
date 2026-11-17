package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	EntryCredit = "CREDIT"
	EntryDebit  = "DEBIT"

	ReasonTokenPurchase = "TOKEN_PURCHASE"
	ReasonSpend         = "SPEND"
	ReasonAppFund       = "APP_FUND"
)

type LedgerEntry struct {
	bun.BaseModel `bun:"table:ledger_entries,alias:le"`

	ID             uuid.UUID      `bun:"id,pk,type:uuid"`
	TransactionID  uuid.UUID      `bun:"transaction_id,notnull,type:uuid"`
	WalletID       uuid.UUID      `bun:"wallet_id,notnull,type:uuid"`
	AppID          string         `bun:"app_id,notnull"`
	EntryType      string         `bun:"entry_type,notnull"`
	Reason         string         `bun:"reason,notnull"`
	Amount         int64          `bun:"amount,notnull"`
	BalanceBefore  int64          `bun:"balance_before,notnull"`
	BalanceAfter   int64          `bun:"balance_after,notnull"`
	IdempotencyKey string         `bun:"idempotency_key,notnull"`
	RequestHash    string         `bun:"request_hash,notnull"`
	OrderID        string         `bun:"order_id,notnull"`
	Metadata       map[string]any `bun:"metadata,type:jsonb"`
	CreatedAt      time.Time      `bun:"created_at,notnull"`
}
