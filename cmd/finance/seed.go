package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/pakatagoh/finance/internal/categories"
	"github.com/pakatagoh/finance/internal/config"
	"github.com/pakatagoh/finance/internal/storage"
)

const categorySeedTimeout = 1 * time.Minute

func seedCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) != 1 || args[0] != "initial-categories" {
		return fmt.Errorf("usage: finance seed initial-categories")
	}
	cfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	seedCtx, cancel := context.WithTimeout(ctx, categorySeedTimeout)
	defer cancel()
	pool, err := storage.NewPool(seedCtx, cfg)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	completed, err := categories.SeedInitialCategories(seedCtx, pool)
	if err != nil {
		return err
	}
	if completed {
		_, _ = fmt.Fprintln(out, "initial categories seeded")
	} else {
		_, _ = fmt.Fprintln(out, "initial categories already completed")
	}
	return nil
}
