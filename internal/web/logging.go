package web

import (
	"log/slog"
	"net/http"
	"time"
)

// RequestLogging records only operational metadata. It deliberately excludes
// headers, query strings, request bodies, and response bodies.
func RequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		path := r.Pattern
		if path == "" {
			path = "[unmatched]"
		}
		logger.Info("http request", "method", r.Method, "route", path, "status", rw.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}
