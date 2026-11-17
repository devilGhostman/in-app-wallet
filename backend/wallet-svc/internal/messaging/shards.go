package messaging

import (
	"fmt"
	"strings"

	"wallet-svc/internal/config"
)

// ShardsForReplica returns shard IDs owned by replica index in a set of count replicas
// over total shard queues: shardID % count == index.
func ShardsForReplica(total, index, count int) ([]int, error) {
	if total <= 0 {
		return nil, fmt.Errorf("shard total must be > 0")
	}
	if count <= 0 {
		return nil, fmt.Errorf("shard count must be > 0")
	}
	if count > total {
		return nil, fmt.Errorf("shard count %d exceeds total %d", count, total)
	}
	if index < 0 || index >= count {
		return nil, fmt.Errorf("shard index %d out of range [0, %d)", index, count)
	}
	out := make([]int, 0, (total/count)+1)
	for id := 0; id < total; id++ {
		if id%count == index {
			out = append(out, id)
		}
	}
	return out, nil
}

// QueueName builds a shard queue name: {prefix}.{shardID}.
func QueueName(prefix string, shardID int) string {
	return fmt.Sprintf("%s.%d", strings.TrimSpace(prefix), shardID)
}

// AssignedQueues returns queue names this replica should consume.
func AssignedQueues(cfg config.RabbitMQConfig) ([]string, error) {
	ids, err := ShardsForReplica(cfg.ShardTotal, cfg.ShardIndex, cfg.ShardCount)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, QueueName(cfg.QueuePrefix, id))
	}
	return out, nil
}
