package server

import (
	"wallet-svc/internal/config"
	"wallet-svc/internal/database"
	"wallet-svc/internal/metrics"
	"wallet-svc/internal/repository"
	"wallet-svc/internal/services"

	"go.uber.org/zap"
)

// WalletDeps holds shared dependencies for HTTP routes and RabbitMQ consumers.
type WalletDeps struct {
	Service  *services.WalletService
	Repo     repository.WalletRepository
	Counters *metrics.Counters
	Limiter  repository.RateLimiter
}

func NewWalletDeps(cfg *config.Config, log *zap.Logger, db *database.PostgresDB, redis *database.RedisClient) WalletDeps {
	walletRepo := repository.NewWalletRepository(db.DB, cfg.App.AppWalletShards)
	var limiter repository.RateLimiter
	var cache repository.BalanceCache
	if redis != nil && redis.Client != nil {
		limiter = repository.NewRedisRateLimiter(redis.Client)
		cache = repository.NewRedisBalanceCache(redis.Client)
	}
	counters := &metrics.Counters{}
	svc := services.NewWalletService(walletRepo, cache, cfg.Limits, cfg.DB, cfg.App.UserIDPattern, counters, log)
	return WalletDeps{Service: svc, Repo: walletRepo, Counters: counters, Limiter: limiter}
}
