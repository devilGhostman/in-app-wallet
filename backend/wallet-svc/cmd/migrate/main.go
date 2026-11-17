package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"wallet-svc/internal/config"
	"wallet-svc/internal/database"
	"wallet-svc/internal/logger"

	"go.uber.org/zap"
)

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

	dbCfg := cfg.DB
	if dbCfg.MigrateURL != "" {
		dbCfg.URL = dbCfg.MigrateURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := database.NewPostgresDB(ctx, dbCfg, log)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	if err := database.MigrateUp(ctx, db, log); err != nil {
		log.Fatal("Migration failed", zap.Error(err))
	}
}
