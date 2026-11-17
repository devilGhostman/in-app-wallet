package database

import (
	"context"
	"fmt"
	"time"

	"order-svc/internal/config"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisClient struct {
	Client *redis.Client
	logger *zap.Logger
}

func NewRedisClient(ctx context.Context, cfg config.RedisConfig, log *zap.Logger) (*RedisClient, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	log.Info("Redis connection established",
		zap.Int("pool_size", opts.PoolSize),
		zap.Int("min_idle_conns", opts.MinIdleConns),
	)

	return &RedisClient{Client: client, logger: log}, nil
}

func (r *RedisClient) Ping(ctx context.Context) error {
	if r == nil || r.Client == nil {
		return fmt.Errorf("redis unconfigured")
	}
	return r.Client.Ping(ctx).Err()
}

func (r *RedisClient) Close() error {
	if r == nil || r.Client == nil {
		return nil
	}
	r.logger.Info("Closing Redis connection")
	return r.Client.Close()
}
