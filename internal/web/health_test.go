package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type healthDB struct {
	pingErr    error
	version    int64
	versionErr error
}

func (d healthDB) Ping(context.Context) error                  { return d.pingErr }
func (d healthDB) GooseVersion(context.Context) (int64, error) { return d.version, d.versionErr }

func TestHealthEndpoints(t *testing.T) {
	h := NewHealthHandler(healthDB{version: 3}, 3)
	h.MarkStartupComplete()
	for _, tc := range []struct {
		name, path string
		want       int
	}{
		{"live", "/health/live", http.StatusOK}, {"ready", "/health/ready", http.StatusOK}, {"startup", "/health/startup", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d body=%s", w.Code, tc.want, w.Body)
			}
		})
	}
}

func TestHealthReadinessRequiresDatabaseAndMigrationVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		db   healthDB
		want int
	}{
		{"database unavailable", healthDB{pingErr: errors.New("down"), version: 3}, http.StatusServiceUnavailable},
		{"migration query unavailable", healthDB{versionErr: errors.New("missing table")}, http.StatusServiceUnavailable},
		{"migration behind", healthDB{version: 2}, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHealthHandler(tc.db, 3)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d", w.Code, tc.want)
			}
		})
	}
}

func TestStartupIsUnavailableUntilComplete(t *testing.T) {
	h := NewHealthHandler(healthDB{version: 3}, 3)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/startup", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
}
