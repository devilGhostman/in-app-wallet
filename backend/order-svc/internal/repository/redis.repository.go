package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const balanceCacheTTL = 30 * time.Second

type BalanceCache interface {
	Get(ctx context.Context, appID, userID string) (balance int64, version int64, ok bool, err error)
	SetIfNewer(ctx context.Context, appID, userID string, balance, version int64) error
}

type redisBalanceCache struct {
	client *redis.Client
}

func NewRedisBalanceCache(client *redis.Client) BalanceCache {
	return &redisBalanceCache{client: client}
}

type cachedBalance struct {
	Balance int64 `json:"balance"`
	Version int64 `json:"version"`
}

func balanceKey(appID, userID string) string {
	return fmt.Sprintf("wallet:balance:%s:%s", appID, userID)
}

var setIfNewerScript = redis.NewScript(`
local key = KEYS[1]
local incoming = cjson.decode(ARGV[1])
local ttl = tonumber(ARGV[2])
local raw = redis.call('GET', key)
if raw then
  local current = cjson.decode(raw)
  if current.version ~= nil and incoming.version <= current.version then
    return 0
  end
end
redis.call('SET', key, ARGV[1], 'EX', ttl)
return 1
`)

func (c *redisBalanceCache) Get(ctx context.Context, appID, userID string) (int64, int64, bool, error) {
	if c.client == nil {
		return 0, 0, false, nil
	}
	raw, err := c.client.Get(ctx, balanceKey(appID, userID)).Bytes()
	if err == redis.Nil {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	var cached cachedBalance
	if err := json.Unmarshal(raw, &cached); err != nil {
		return 0, 0, false, err
	}
	return cached.Balance, cached.Version, true, nil
}

func (c *redisBalanceCache) SetIfNewer(ctx context.Context, appID, userID string, balance, version int64) error {
	if c.client == nil {
		return nil
	}
	payload, err := json.Marshal(cachedBalance{Balance: balance, Version: version})
	if err != nil {
		return err
	}
	return setIfNewerScript.Run(ctx, c.client, []string{balanceKey(appID, userID)}, payload, strconv.Itoa(int(balanceCacheTTL.Seconds()))).Err()
}
