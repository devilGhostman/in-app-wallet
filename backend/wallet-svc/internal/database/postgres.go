package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"wallet-svc/internal/config"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"go.uber.org/zap"
)

type PostgresDB struct {
	DB     *bun.DB
	sqlDB  *sql.DB
	logger *zap.Logger
}

func NewPostgresDB(ctx context.Context, cfg config.DBConfig, log *zap.Logger) (*PostgresDB, error) {
	sqlDB, err := sql.Open("pgx", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection pool: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	bunDB := bun.NewDB(sqlDB, pgdialect.New())

	log.Info("PostgreSQL connection pool established",
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
	)

	return &PostgresDB{
		DB:     bunDB,
		sqlDB:  sqlDB,
		logger: log,
	}, nil
}

func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.sqlDB.PingContext(ctx)
}

func (p *PostgresDB) Close() error {
	p.logger.Info("Closing PostgreSQL connection pool")
	return p.DB.Close()
}
