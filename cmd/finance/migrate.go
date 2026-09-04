package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pakatagoh/finance/internal/config"
	"github.com/pakatagoh/finance/internal/migrations"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

const (
	migrationTimeout = 5 * time.Minute
	migrationLockID  = int64(0x66696e616e6365) // stable Finance namespace
)

// runMigrations opens one database/sql pool and lets Goose own version
// tracking and migration transactions. The session locker acquires a
// PostgreSQL advisory lock on the same session used for the migration run.
func runMigrations(ctx context.Context, databaseURL string, fsys embed.FS) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(migrationLockID))
	if err != nil {
		return fmt.Errorf("create migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys,
		goose.WithSessionLocker(locker),
		goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	defer provider.Close()
	if err := provider.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func migrateCommand(ctx context.Context, out, errOut io.Writer) error {
	cfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	migrationCtx, cancel := context.WithTimeout(ctx, migrationTimeout)
	defer cancel()
	if err := runMigrations(migrationCtx, cfg.DatabaseURL, migrations.FS); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "migrations applied")
	return nil
}
