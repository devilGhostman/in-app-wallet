package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"gateway-svc/internal/config"
	"gateway-svc/internal/database"
	"gateway-svc/internal/logger"
	"gateway-svc/internal/metrics"
	"gateway-svc/internal/proxy"
	"gateway-svc/internal/repository"
	"gateway-svc/internal/server"

	"go.uber.org/zap"
)

// @title           Gateway Service API
// @version         1.0
// @description     Edge gateway: JWT auth, rate limit, reverse proxy to wallet-svc and order-svc.
// @host            localhost:8081
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.NewLogger(cfg.Log, cfg.App.Env)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	ctx := context.Background()

	redis, err := database.NewRedisClient(ctx, cfg.Redis, log)
	if err != nil {
		log.Fatal("Failed to connect Redis", zap.Error(err))
	}
	defer func() { _ = redis.Close() }()

	limiter := repository.NewRedisRateLimiter(redis.Client)
	m := metrics.New()

	proxyRouter, err := proxy.NewRouter(cfg, m)
	if err != nil {
		log.Fatal("Failed to build proxy router", zap.Error(err))
	}

	srv, err := server.NewServer(cfg, log, redis, limiter, m, proxyRouter)
	if err != nil {
		log.Fatal("Failed to create server", zap.Error(err))
	}

	errCh := srv.Start()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal("Server stopped with error", zap.Error(err))
		}
	case sig := <-sigCh:
		log.Info("Shutdown signal received", zap.String("signal", sig.String()))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("Graceful shutdown failed", zap.Error(err))
		}
	}
}
