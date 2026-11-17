package messaging

import (
	"testing"

	"wallet-svc/internal/model"

	"github.com/stretchr/testify/require"
)

func TestParseAndValidateCreditCommand(t *testing.T) {
	raw := []byte(`{
		"app_id":"app-1",
		"user_id":"aaaaaaaaaaaaaaaaaaaaaaaa",
		"amount":100,
		"reason":"TOKEN_PURCHASE",
		"order_id":"ord-1",
		"idempotency_key":"order:ord-1:credit"
	}`)
	cmd, err := ParseCommand(raw)
	require.NoError(t, err)
	require.NoError(t, cmd.Validate())
	op, err := OpFromReason(cmd.Reason)
	require.NoError(t, err)
	require.Equal(t, OpCredit, op)
	require.Equal(t, model.ReasonTokenPurchase, cmd.Reason)
	req := cmd.ToMutationRequest()
	require.Equal(t, int64(100), req.Amount)
	require.Equal(t, "order:ord-1:credit", req.IdempotencyKey)
}

func TestValidateRejectsUnknownReason(t *testing.T) {
	cmd := WalletMutationCommand{
		AppID: "app-1", UserID: "u1", Amount: 1,
		Reason: "OTHER", OrderID: "o1", IdempotencyKey: "order:o1:credit",
	}
	require.Error(t, cmd.Validate())
}

func TestValidateDebit(t *testing.T) {
	cmd := WalletMutationCommand{
		AppID: "app-1", UserID: "u1", Amount: 5,
		Reason: model.ReasonSpend, OrderID: "o1", IdempotencyKey: "order:o1:debit",
	}
	require.NoError(t, cmd.Validate())
	op, err := OpFromReason(cmd.Reason)
	require.NoError(t, err)
	require.Equal(t, OpDebit, op)
}

func TestOpFromReason(t *testing.T) {
	op, err := OpFromReason(model.ReasonTokenPurchase)
	require.NoError(t, err)
	require.Equal(t, OpCredit, op)
	op, err = OpFromReason(model.ReasonSpend)
	require.NoError(t, err)
	require.Equal(t, OpDebit, op)
	_, err = OpFromReason("")
	require.Error(t, err)
}

func TestCorrelationIDFallsBackToIdempotencyKey(t *testing.T) {
	cmd := WalletMutationCommand{IdempotencyKey: "order:o1:credit"}
	require.Equal(t, "order:o1:credit", cmd.CorrelationID())
	cmd.RequestID = "req-9"
	require.Equal(t, "req-9", cmd.CorrelationID())
}
