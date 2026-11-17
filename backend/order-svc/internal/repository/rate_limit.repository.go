package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Take(ctx context.Context, key string, capacity int, refill time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

var tokenBucketScript = redis.NewScript(`
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local b = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(b[1]) or capacity
local ts = tonumber(b[2]) or now
tokens = math.min(capacity, tokens + (now - ts) * rate)
local allowed, retry = 0, 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  retry = math.ceil((1 - tokens) / rate)
end
redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', now)
redis.call('PEXPIRE', KEYS[1], math.ceil(capacity / rate))
return {allowed, retry}
`)

type redisRateLimiter struct {
	client *redis.Client
}

func NewRedisRateLimiter(client *redis.Client) RateLimiter {
	return &redisRateLimiter{client: client}
}

func (r *redisRateLimiter) Take(ctx context.Context, key string, capacity int, refill time.Duration) (bool, time.Duration, error) {
	if r.client == nil {
		return false, 0, errors.New("redis client is not initialized")
	}
	refillMs := refill / time.Millisecond
	if capacity < 1 || refillMs < 1 {
		return false, 0, fmt.Errorf("invalid token bucket: capacity=%d refill=%s", capacity, refill)
	}

	rate := 1.0 / float64(refillMs)
	res, err := tokenBucketScript.Run(ctx, r.client, []string{key}, capacity, strconv.FormatFloat(rate, 'f', 12, 64)).Result()
	if err != nil {
		return false, 0, fmt.Errorf("token bucket eval: %w", err)
	}
	vals, ok := res.([]any)
	if !ok || len(vals) != 2 {
		return false, 0, fmt.Errorf("token bucket eval: unexpected reply %v", res)
	}
	allowed, err := toInt64(vals[0])
	if err != nil {
		return false, 0, err
	}
	retry, err := toInt64(vals[1])
	if err != nil {
		return false, 0, err
	}
	return allowed == 1, time.Duration(retry) * time.Millisecond, nil
}

func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case string:
		return strconv.ParseInt(n, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected int type %T", v)
	}
}
