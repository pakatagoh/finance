package web

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// HealthDB is the minimal database surface needed for readiness checks.
type HealthDB interface {
	Ping(context.Context) error
	GooseVersion(context.Context) (int64, error)
}

// HealthHandler serves process and dependency health probes.
type HealthHandler struct {
	db              HealthDB
	expectedVersion int64
	startupComplete atomic.Bool
}

func NewHealthHandler(db HealthDB, expectedVersion int64) *HealthHandler {
	return &HealthHandler{db: db, expectedVersion: expectedVersion}
}

// MarkStartupComplete marks initialization complete and enables the startup probe.
func (h *HealthHandler) MarkStartupComplete() { h.startupComplete.Store(true) }

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health/live":
		healthJSON(w, http.StatusOK, "ok")
	case "/health/ready":
		if h.db == nil || h.db.Ping(r.Context()) != nil {
			healthJSON(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		version, err := h.db.GooseVersion(r.Context())
		if err != nil || version != h.expectedVersion {
			healthJSON(w, http.StatusServiceUnavailable, "migrations incomplete")
			return
		}
		healthJSON(w, http.StatusOK, "ok")
	case "/health/startup":
		if !h.startupComplete.Load() {
			healthJSON(w, http.StatusServiceUnavailable, "startup incomplete")
			return
		}
		healthJSON(w, http.StatusOK, "ok")
	default:
		http.NotFound(w, r)
	}
}

func healthJSON(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{value})
}
