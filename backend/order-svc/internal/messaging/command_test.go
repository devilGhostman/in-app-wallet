package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandFromOutboxPayload(t *testing.T) {
	cmd, err := CommandFromOutboxPayload(map[string]any{
		"app_id":          "app-1",
		"user_id":         "u1",
		"amount":          float64(100),
		"reason":          "TOKEN_PURCHASE",
		"order_id":        "ord-1",
		"idempotency_key": "order:ord-1:credit",
		"metadata":        map[string]any{"k": "v"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(100), cmd.Amount)
	require.Equal(t, "TOKEN_PURCHASE", cmd.Reason)
	require.Equal(t, "v", cmd.Metadata["k"])
}

func TestCommandFromOutboxPayloadRejectsIncomplete(t *testing.T) {
	_, err := CommandFromOutboxPayload(map[string]any{"app_id": "a"})
	require.Error(t, err)
}
