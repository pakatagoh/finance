// Package storage owns database pool creation and shared database helpers.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pakatagoh/finance/internal/config"
)

// Health exposes only safe dependency checks to HTTP health probes.
type Health struct{ pool *pgxpool.Pool }

func NewHealth(pool *pgxpool.Pool) Health       { return Health{pool: pool} }
func (h Health) Ping(ctx context.Context) error { return Ping(ctx, h.pool) }
func (h Health) GooseVersion(ctx context.Context) (int64, error) {
	var version int64
	err := h.pool.QueryRow(ctx, "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version").Scan(&version)
	return version, err
}

const (
	maxConns          int32 = 10
	minConns          int32 = 1
	connLifetime            = time.Hour
	connIdleTime            = 15 * time.Minute
	healthCheckPeriod       = time.Minute
	pingTimeout             = 5 * time.Second
)

// NewPoolConfig parses the database URL and applies the application's safe,
// bounded pool defaults. It does not connect to the database.
func NewPoolConfig(cfg config.Config) (*pgxpool.Config, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	poolCfg.MinConns = minConns
	poolCfg.MaxConns = maxConns
	poolCfg.MaxConnLifetime = connLifetime
	poolCfg.MaxConnIdleTime = connIdleTime
	poolCfg.HealthCheckPeriod = healthCheckPeriod
	// Keep timestamp behavior independent of the database server's default.
	poolCfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	return poolCfg, nil
}

// NewPool creates a bounded pool and verifies connectivity before returning it.
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := NewPoolConfig(cfg)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := Ping(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Pinger is the public subset needed by Ping.
type Pinger interface {
	Ping(context.Context) error
}

// Ping checks database connectivity with a bounded timeout.
func Ping(ctx context.Context, p Pinger) error {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return p.Ping(pingCtx)
}
