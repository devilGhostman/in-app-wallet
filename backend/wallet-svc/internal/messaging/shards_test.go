package messaging

import (
	"testing"

	"wallet-svc/internal/config"

	"github.com/stretchr/testify/require"
)

func TestShardsForReplica(t *testing.T) {
	ids, err := ShardsForReplica(16, 0, 4)
	require.NoError(t, err)
	require.Equal(t, []int{0, 4, 8, 12}, ids)

	ids, err = ShardsForReplica(16, 1, 4)
	require.NoError(t, err)
	require.Equal(t, []int{1, 5, 9, 13}, ids)

	ids, err = ShardsForReplica(16, 0, 1)
	require.NoError(t, err)
	require.Len(t, ids, 16)
	require.Equal(t, 0, ids[0])
	require.Equal(t, 15, ids[15])
}

func TestShardsForReplicaRejectsBadArgs(t *testing.T) {
	_, err := ShardsForReplica(0, 0, 1)
	require.Error(t, err)
	_, err = ShardsForReplica(16, 4, 4)
	require.Error(t, err)
	_, err = ShardsForReplica(8, 0, 16)
	require.Error(t, err)
}

func TestAssignedQueues(t *testing.T) {
	queues, err := AssignedQueues(config.RabbitMQConfig{
		QueuePrefix: "wallet.commands",
		ShardTotal:  16,
		ShardIndex:  0,
		ShardCount:  4,
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"wallet.commands.0",
		"wallet.commands.4",
		"wallet.commands.8",
		"wallet.commands.12",
	}, queues)
}

func TestQueueName(t *testing.T) {
	require.Equal(t, "wallet.commands.3", QueueName("wallet.commands", 3))
}
