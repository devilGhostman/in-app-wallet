package messaging

import (
	"testing"

	"wallet-svc/pkg/response"

	"github.com/stretchr/testify/require"
)

func TestFailureEventFromCommandDebit(t *testing.T) {
	cmd := WalletMutationCommand{
		AppID: "app", UserID: "u", Amount: 10, Reason: "SPEND",
		OrderID: "ord-1", IdempotencyKey: "order:ord-1:debit",
	}
	ev, err := FailureEventFromCommand(cmd, OpDebit, response.Unprocessable(response.CodeInsufficientFunds, "not enough"))
	require.NoError(t, err)
	require.Equal(t, EventDebitFailed, ev.EventType)
	require.Equal(t, "INSUFFICIENT_FUNDS", ev.Code)
	require.Equal(t, "not enough", ev.Message)
	require.Equal(t, "ord-1", ev.OrderID)
}

func TestFailureEventFromCommandCredit(t *testing.T) {
	cmd := WalletMutationCommand{
		AppID: "app", UserID: "u", Amount: 5, Reason: "TOKEN_PURCHASE",
		OrderID: "ord-2", IdempotencyKey: "order:ord-2:credit",
	}
	ev, err := FailureEventFromCommand(cmd, OpCredit, response.Unprocessable(response.CodeLimitExceeded, "cap"))
	require.NoError(t, err)
	require.Equal(t, EventCreditFailed, ev.EventType)
	require.Equal(t, "LIMIT_EXCEEDED", ev.Code)
}

func TestLifecycleEventFromOutbox(t *testing.T) {
	ev := LifecycleEventFromOutbox(EventCredited, map[string]any{
		"app_id": "a", "user_id": "u", "order_id": "o", "amount": float64(9),
		"idempotency_key": "k", "reason": "TOKEN_PURCHASE",
	})
	require.Equal(t, EventCredited, ev.EventType)
	require.Equal(t, int64(9), ev.Amount)
	require.Equal(t, "o", ev.OrderID)
}
