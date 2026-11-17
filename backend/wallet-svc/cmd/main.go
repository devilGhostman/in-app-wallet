package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"wallet-svc/internal/config"
	"wallet-svc/internal/database"
	"wallet-svc/internal/logger"
	"wallet-svc/internal/messaging"
	"wallet-svc/internal/server"

	"go.uber.org/zap"
)

// @title           wallet-svc API
// @version         1.0
// @description     Closed-loop in-app coin wallet with double-entry ledger. Internal service APIs only. Send Authorization: Bearer &lt;HS256 JWT signed with JWT_SECRET&gt; (claims: exp), required X-Scopes and X-App-Id, optional X-Request-ID (correlation).
// @description
// @description     RabbitMQ AMQP 1.0 (when RABBITMQ_ENABLED=true): Orders publishes credit/debit commands to x-consistent-hash exchange wallet.commands (RK=user_id). Op from reason (TOKEN_PURCHASE→credit, SPEND→debit). Success publishes wallet.credited|debited via outbox to wallet.events; permanent failures publish wallet.credit_failed|debit_failed then DLQ. HTTP credit/debit routes remain available. See tag "messaging".
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description     Bearer HS256 JWT signed with JWT_SECRET, e.g. "Bearer eyJ..."
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
	defer func() {
		_ = log.Sync()
	}()

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
		log.Warn("Redis unavailable; continuing without cache or rate limit", zap.Error(err))
		redisClient = nil
	} else {
		defer func() {
			if closeErr := redisClient.Close(); closeErr != nil {
				log.Error("Error closing Redis connection", zap.Error(closeErr))
			}
		}()
	}

	deps := server.NewWalletDeps(cfg, log, db, redisClient)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	mqConsumer := messaging.NewConsumer(cfg.RabbitMQ, messaging.NewWalletMutationHandler(deps.Service), log)
	mqConsumer.Start(runCtx)
	defer mqConsumer.Stop()

	outboxPub := messaging.NewOutboxPublisher(cfg.RabbitMQ, deps.Repo, log)
	outboxPub.Start(runCtx)
	defer outboxPub.Stop()

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
		outboxPub.Stop()
		mqConsumer.Stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("Server shutdown error", zap.Error(err))
			_ = srv.Shutdown(context.Background())
		}
		log.Info("Server stopped cleanly")
	}
}
