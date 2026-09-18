package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"github.com/pakatagoh/finance/internal/categorization"
	"github.com/pakatagoh/finance/internal/config"
	"github.com/pakatagoh/finance/internal/jev"
	"github.com/pakatagoh/finance/internal/storage"
)

func categorizeCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) != 0 && (len(args) != 2 || args[0] != "--limit") {
		return fmt.Errorf("usage: finance categorize [--limit N]")
	}
	cfg, err := config.LoadCategorization()
	if err != nil {
		return err
	}
	limit := cfg.CategorizeBatchLimit
	if len(args) == 2 {
		limit, err = strconv.Atoi(args[1])
		if err != nil || limit <= 0 {
			return fmt.Errorf("categorize limit must be positive")
		}
	}
	pool, err := storage.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	client, err := jev.NewClient(cfg.JevEndpoint, cfg.JevToken)
	if err != nil {
		return err
	}
	store := storage.CategorizationStore{Pool: pool}
	service := categorization.NewServiceWithThreshold(categorization.NewStorageMappingStore(store), categorization.NewJevCategorizer(client, cfg.JevModel), cfg.CategorizeThreshold)
	processed, failed, err := categorization.RunBatch(ctx, store, service, cfg.JevModel, limit, slog.New(slog.NewJSONHandler(errOut, nil)))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "categorized %d transactions (%d failed)\n", processed, failed)
	return nil
}
