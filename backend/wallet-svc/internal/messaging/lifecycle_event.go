package messaging

import (
	"encoding/json"
	"errors"
	"strings"

	"wallet-svc/pkg/response"
)

const (
	EventCredited     = "wallet.credited"
	EventDebited      = "wallet.debited"
	EventCreditFailed = "wallet.credit_failed"
	EventDebitFailed  = "wallet.debit_failed"
)

// LifecycleEvent is published to wallet.events for order-svc.
type LifecycleEvent struct {
	EventType      string `json:"event_type"`
	AppID          string `json:"app_id"`
	UserID         string `json:"user_id"`
	OrderID        string `json:"order_id"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func LifecycleEventFromOutbox(eventType string, payload map[string]any) LifecycleEvent {
	return LifecycleEvent{
		EventType:      eventType,
		AppID:          asString(payload["app_id"]),
		UserID:         asString(payload["user_id"]),
		OrderID:        asString(payload["order_id"]),
		Amount:         asInt64(payload["amount"]),
		IdempotencyKey: asString(payload["idempotency_key"]),
		Reason:         asString(payload["reason"]),
	}
}

func FailureEventFromCommand(cmd WalletMutationCommand, op MutationOp, cause error) (LifecycleEvent, error) {
	eventType := EventCreditFailed
	switch {
	case op == OpDebit:
		eventType = EventDebitFailed
	case op == OpCredit:
		eventType = EventCreditFailed
	default:
		if inferred, err := OpFromReason(cmd.Reason); err == nil && inferred == OpDebit {
			eventType = EventDebitFailed
		} else if strings.HasSuffix(cmd.IdempotencyKey, ":debit") {
			eventType = EventDebitFailed
		}
	}

	code, message := "PERMANENT_FAILURE", "wallet command permanently failed"
	var appErr *response.AppError
	if errors.As(cause, &appErr) {
		if appErr.Code != "" {
			code = appErr.Code
		}
		if appErr.Message != "" {
			message = appErr.Message
		}
	} else if cause != nil {
		message = cause.Error()
	}

	return LifecycleEvent{
		EventType:      eventType,
		AppID:          cmd.AppID,
		UserID:         cmd.UserID,
		OrderID:        cmd.OrderID,
		Amount:         cmd.Amount,
		IdempotencyKey: cmd.IdempotencyKey,
		Code:           code,
		Message:        message,
		Reason:         cmd.Reason,
	}, nil
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
