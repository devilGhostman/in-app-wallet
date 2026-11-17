package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"order-svc/internal/config"
	"order-svc/internal/database"
	"order-svc/internal/logger"
	"order-svc/internal/messaging"
	"order-svc/internal/server"

	"go.uber.org/zap"
)

// @title           order-svc API
// @version         1.0
// @description     Order service for purchase (PSP → paid → TOKEN_PURCHASE credit) and spend (no gateway → SPEND debit). Terminal status comes from wallet result events (wallet.credited|debited|credit_failed|debit_failed) on queue order.wallet.lifecycle.
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description     Bearer HS256 JWT signed with JWT_SECRET
func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("FATAL: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewLogger(cfg.Log, cfg.App.Env)
	if err != nil {
		fmt.Printf("FATAL: Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting application",
		zap.String("app", cfg.App.Name),
		zap.String("env", cfg.App.Env),
		zap.Int("port", cfg.App.Port),
	)

	initCtx, initCancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer initCancel()

	db, err := database.NewPostgresDB(initCtx, cfg.DB, log)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Error("Error closing PostgreSQL connection", zap.Error(closeErr))
		}
	}()

	var redisClient *database.RedisClient
	redisClient, err = database.NewRedisClient(initCtx, cfg.Redis, log)
	if err != nil {
		log.Warn("Redis unavailable; continuing without rate limit", zap.Error(err))
		redisClient = nil
	} else {
		defer func() {
			if closeErr := redisClient.Close(); closeErr != nil {
				log.Error("Error closing Redis connection", zap.Error(closeErr))
			}
		}()
	}

	deps := server.NewOrderDeps(cfg, log, db, redisClient)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	publisher := messaging.NewOutboxPublisher(cfg.RabbitMQ, deps.Repo, log)
	publisher.Start(runCtx)
	defer publisher.Stop()

	lifecycle := messaging.NewLifecycleConsumer(cfg.RabbitMQ, deps.Orders, log)
	lifecycle.Start(runCtx)
	defer lifecycle.Stop()

	srv, err := server.NewServer(cfg, log, db, redisClient, deps)
	if err != nil {
		log.Fatal("Failed to set up HTTP server", zap.Error(err))
	}
	serverErrors := srv.Start()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serverErrors:
		log.Fatal("Fatal server error occurred", zap.Error(err))
	case sig := <-shutdownSignal:
		log.Info("Shutdown signal received", zap.String("signal", sig.String()))
		runCancel()
		lifecycle.Stop()
		publisher.Stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("Server shutdown error", zap.Error(err))
			_ = srv.Shutdown(context.Background())
		}
		log.Info("Server stopped cleanly")
	}
}
