package server

import (
	"order-svc/internal/config"
	"order-svc/internal/database"
	"order-svc/internal/repository"
	"order-svc/internal/services"

	"go.uber.org/zap"
)

type OrderDeps struct {
	Orders  *services.OrderService
	PaySim  *services.PaymentSimulator
	Repo    repository.OrderRepository
	Limiter repository.RateLimiter
}

func NewOrderDeps(cfg *config.Config, log *zap.Logger, db *database.PostgresDB, redis *database.RedisClient) OrderDeps {
	repo := repository.NewOrderRepository(db.DB)
	orders := services.NewOrderService(repo, cfg.Limits, cfg.App.UserIDPattern, log)
	pay := services.NewPaymentSimulator(repo, cfg.Payment, log)

	var limiter repository.RateLimiter
	if redis != nil {
		limiter = repository.NewRedisRateLimiter(redis.Client)
	}

	return OrderDeps{
		Orders:  orders,
		PaySim:  pay,
		Repo:    repo,
		Limiter: limiter,
	}
}
