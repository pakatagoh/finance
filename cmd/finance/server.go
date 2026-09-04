package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pakatagoh/finance/internal/config"
	"github.com/pakatagoh/finance/internal/migrations"
	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/web"
)

func newHTTPServer(pool *pgxpool.Pool, logger *slog.Logger, token, origin string) (*http.Server, *web.HealthHandler) {
	health := web.NewHealthHandler(storage.NewHealth(pool), migrations.LatestVersion)
	uiMux := http.NewServeMux()
	uiMux.Handle("GET /health/live", health)
	uiMux.Handle("GET /health/ready", health)
	uiMux.Handle("GET /health/startup", health)
	uiStore := storage.TransactionStore{Pool: pool}
	uiMux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	uiMux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/transactions", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	uiMux.Handle("GET /transactions", web.TransactionsHandler(uiStore))
	uiMux.Handle("GET /transactions/{uuid}", web.TransactionDetailHandler(uiStore))
	uiMux.Handle("POST /transactions/{uuid}", web.TransactionDetailHandler(uiStore))
	uiMux.Handle("PATCH /transactions/{uuid}", web.TransactionDetailHandler(uiStore))
	root := http.NewServeMux()
	root.Handle("/api/v1/transactions", web.BearerAPI(token, web.CreateTransactionHandlerWithLogger(uiStore, logger)))
	root.Handle("/", web.UIHandler(origin, uiMux))
	return &http.Server{Addr: ":8080", Handler: web.RequestLogging(logger, root), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}, health
}

func serveCommand(ctx context.Context, out, errOut io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := storage.NewPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger := slog.New(slog.NewJSONHandler(errOut, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server, health := newHTTPServer(pool, logger, cfg.APIToken, cfg.AppOrigin)
	health.MarkStartupComplete()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server stopped", "error", "server failure")
		}
	}()
	_, _ = fmt.Fprintln(out, "finance listening on :8080")
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func runServe(out, errOut io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveCommand(ctx, out, errOut)
}
