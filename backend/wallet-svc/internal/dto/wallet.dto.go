package dto

type CreateWalletRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type MutationRequest struct {
	Amount         int64          `json:"amount" binding:"required"`
	Reason         string         `json:"reason" binding:"required"`
	OrderID        string         `json:"order_id" binding:"required"`
	IdempotencyKey string         `json:"idempotency_key" binding:"required"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type FundAppRequest struct {
	Amount         int64          `json:"amount" binding:"required"`
	OrderID        string         `json:"order_id" binding:"required"`
	IdempotencyKey string         `json:"idempotency_key" binding:"required"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type MutationResponse struct {
	AppID         string `json:"app_id"`
	UserID        string `json:"user_id,omitempty"`
	Amount        int64  `json:"amount"`
	Balance       int64  `json:"balance"`
	LedgerID      string `json:"ledger_id"`
	TransactionID string `json:"transaction_id"`
	Duplicate     bool   `json:"duplicate"`
}

type WalletResponse struct {
	AppID   string `json:"app_id"`
	UserID  string `json:"user_id"`
	Balance int64  `json:"balance"`
	Status  int16  `json:"status"`
	Version int64  `json:"version"`
}

type LedgerItem struct {
	LedgerID       string         `json:"ledger_id"`
	TransactionID  string         `json:"transaction_id"`
	AppID          string         `json:"app_id"`
	EntryType      string         `json:"entry_type"`
	Reason         string         `json:"reason"`
	Amount         int64          `json:"amount"`
	BalanceBefore  int64          `json:"balance_before"`
	BalanceAfter   int64          `json:"balance_after"`
	OrderID        string         `json:"order_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	CreatedAt      string         `json:"created_at"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type LedgerPageResponse struct {
	Transactions []LedgerItem `json:"transactions"`
	NextCursor   string       `json:"next_cursor,omitempty"`
}

// LedgerQuery is the list-ledger query string.
// Sort is created_at direction: "asc" or "desc" (default desc).
type LedgerQuery struct {
	Cursor    string
	Limit     int
	EntryType string
	Reason    string
	Sort      string
}

type FreezeRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type ReconcileResponse struct {
	DriftCount int `json:"drift_count"`
	Drifts     any `json:"drifts"`
}
