package storage

import (
	"context"
	"testing"

	"github.com/pakatagoh/finance/internal/config"
)

type fakePinger struct {
	called  bool
	context context.Context
}

func (f *fakePinger) Ping(ctx context.Context) error {
	f.called = true
	f.context = ctx
	return nil
}

func TestPingUsesBoundedTimeout(t *testing.T) {
	p := &fakePinger{}
	if err := Ping(context.Background(), p); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if !p.called {
		t.Fatal("expected pinger to be called")
	}
	if _, ok := p.context.Deadline(); !ok {
		t.Fatal("expected ping context to have a deadline")
	}
}

func TestPoolConfigIsBoundedAndUsesUTC(t *testing.T) {
	cfg, err := NewPoolConfig(config.Config{DatabaseURL: "postgres://finance:password@localhost/finance?sslmode=disable"})
	if err != nil {
		t.Fatalf("NewPoolConfig() error = %v", err)
	}
	if cfg.MinConns != 1 || cfg.MaxConns != 10 {
		t.Fatalf("pool bounds = %d..%d, want 1..10", cfg.MinConns, cfg.MaxConns)
	}
	if cfg.ConnConfig.RuntimeParams["timezone"] != "UTC" {
		t.Fatalf("timezone = %q, want UTC", cfg.ConnConfig.RuntimeParams["timezone"])
	}
	if cfg.MaxConnLifetime <= 0 || cfg.MaxConnIdleTime <= 0 || cfg.HealthCheckPeriod <= 0 {
		t.Fatal("expected bounded pool lifetimes and health check period")
	}
}
