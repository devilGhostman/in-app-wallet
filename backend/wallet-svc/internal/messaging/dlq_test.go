package messaging

import (
	"errors"
	"testing"
	"time"

	"wallet-svc/internal/config"

	amqp "github.com/Azure/go-amqp"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildDeadLetterMessage(t *testing.T) {
	contentType := "application/json"
	src := &amqp.Message{
		Data: [][]byte{[]byte(`{"app_id":"a"}`)},
		Properties: &amqp.MessageProperties{
			MessageID:     "msg-1",
			CorrelationID: "corr-1",
			ContentType:   &contentType,
		},
		ApplicationProperties: map[string]any{
			"trace_id": "t-1",
		},
	}
	failedAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	out := buildDeadLetterMessage(src, "wallet.commands.3", "wallet.commands.dlq", OpCredit, errors.New("bad amount"), failedAt)

	require.Equal(t, []byte(`{"app_id":"a"}`), out.GetData())
	require.Equal(t, "msg-1", out.Properties.MessageID)
	require.Equal(t, "corr-1", out.Properties.CorrelationID)
	require.Equal(t, "wallet.commands.3", out.ApplicationProperties["x-original-queue"])
	require.Equal(t, "wallet.commands.dlq", out.ApplicationProperties["x-dlq"])
	require.Equal(t, "credit", out.ApplicationProperties["x-op"])
	require.Equal(t, "bad amount", out.ApplicationProperties["x-error"])
	require.Equal(t, "t-1", out.ApplicationProperties["trace_id"])
	require.Equal(t, failedAt.Format(time.RFC3339Nano), out.ApplicationProperties["x-failed-at"])
}

func TestConsumerUsesSharedDLQ(t *testing.T) {
	c := NewConsumer(config.RabbitMQConfig{
		CommandsDLQ: "wallet.commands.dlq",
		QueuePrefix: "wallet.commands",
		ShardTotal:  16,
		ShardIndex:  0,
		ShardCount:  1,
	}, nil, zap.NewNop())
	require.Equal(t, "wallet.commands.dlq", c.cfg.CommandsDLQ)
}
