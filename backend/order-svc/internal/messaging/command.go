package messaging

import (
	"encoding/json"
	"fmt"
)

// WalletCreditCommand matches wallet-svc WalletMutationCommand for TOKEN_PURCHASE.
type WalletCreditCommand struct {
	AppID          string         `json:"app_id"`
	UserID         string         `json:"user_id"`
	Amount         int64          `json:"amount"`
	Reason         string         `json:"reason"`
	OrderID        string         `json:"order_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	RequestID      string         `json:"request_id,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

func CommandFromOutboxPayload(payload map[string]any) (WalletCreditCommand, error) {
	if payload == nil {
		return WalletCreditCommand{}, fmt.Errorf("empty payload")
	}
	cmd := WalletCreditCommand{
		AppID:          asString(payload["app_id"]),
		UserID:         asString(payload["user_id"]),
		Reason:         asString(payload["reason"]),
		OrderID:        asString(payload["order_id"]),
		IdempotencyKey: asString(payload["idempotency_key"]),
		RequestID:      asString(payload["request_id"]),
	}
	cmd.Amount = asInt64(payload["amount"])
	if meta, ok := payload["metadata"].(map[string]any); ok {
		cmd.Metadata = meta
	}
	if cmd.AppID == "" || cmd.UserID == "" || cmd.OrderID == "" || cmd.Amount <= 0 {
		return WalletCreditCommand{}, fmt.Errorf("incomplete wallet credit payload")
	}
	return cmd, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}
