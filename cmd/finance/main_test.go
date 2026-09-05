package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pakatagoh/finance/internal/migrations"
)

func TestExecuteRecognizesCommands(t *testing.T) {
	var out, errOut bytes.Buffer
	t.Setenv("DATABASE_URL", "")
	if err := execute([]string{"serve"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("serve error = %v, want missing DATABASE_URL", err)
	}
	if err := execute([]string{"unknown"}, &out, &errOut); err == nil {
		t.Fatal("unknown command unexpectedly succeeded")
	}
}

func TestStaticAssetsAreServed(t *testing.T) {
	t.Chdir("../..")
	server, _ := newHTTPServer(nil, slog.Default(), "token", "http://localhost:8080")
	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("static CSS status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("static CSS content type = %q, want text/css", rec.Header().Get("Content-Type"))
	}
}

func TestTransactionPatchRouteIsRegisteredAndHTMXGated(t *testing.T) {
	server, _ := newHTTPServer(nil, slog.Default(), "token", "http://localhost:8080")
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("notes=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:8080")
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "requires an HTMX request") {
		t.Fatalf("PATCH route/gate: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestEmbeddedMigrationsAreAvailable(t *testing.T) {
	for _, name := range []string{"00001_categories.sql", "00002_transactions.sql", "00003_seed_runs.sql", "00004_card_type.sql", "00005_allow_card_kinds.sql", "00006_remove_card_purchase.sql"} {
		if _, err := migrations.FS.Open(name); err != nil {
			t.Fatalf("embedded migration %q: %v", name, err)
		}
	}
}

func TestLatestMigrationVersionMatchesEmbeddedMigrations(t *testing.T) {
	version, err := migrations.LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if version != 6 {
		t.Fatalf("LatestVersion() = %d, want 6", version)
	}
}

func TestMigrateCommandReportsConfigurationErrors(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var out, errOut bytes.Buffer
	err := migrateCommand(context.Background(), &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("migrate error = %v, want missing DATABASE_URL", err)
	}
}
