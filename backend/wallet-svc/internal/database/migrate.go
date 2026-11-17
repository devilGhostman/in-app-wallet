package database

import (
	"context"
	"fmt"

	"wallet-svc/internal/migrations"

	"github.com/uptrace/bun/migrate"
	"go.uber.org/zap"
)

func NewMigrator(db *PostgresDB) *migrate.Migrator {
	return migrate.NewMigrator(db.DB, migrations.Migrations)
}

func MigrateUp(ctx context.Context, db *PostgresDB, log *zap.Logger) error {
	migrator := NewMigrator(db)
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	if err := migrator.Lock(ctx); err != nil {
		return fmt.Errorf("lock migrator: %w", err)
	}
	defer func() {
		if err := migrator.Unlock(ctx); err != nil {
			log.Error("unlock migrator", zap.Error(err))
		}
	}()

	group, err := migrator.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if group.IsZero() {
		log.Info("No new migrations")
		return nil
	}
	log.Info("Applied migrations", zap.String("group", group.String()))
	return nil
}
