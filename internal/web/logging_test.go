package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLoggingOmitsSecretsAndTransactionDetails(t *testing.T) {
	var logs bytes.Buffer
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })
	h := RequestLogging(slog.New(slog.NewJSONHandler(&logs, nil)), next)
	r := httptest.NewRequest(http.MethodPost, "/transactions/secret-id?amount=999", nil)
	r.Header.Set("Authorization", "Bearer secret-token")
	h.ServeHTTP(httptest.NewRecorder(), r)
	got := logs.String()
	for _, forbidden := range []string{"Authorization", "secret-token", "secret-id", "amount", "999"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log contains forbidden %q: %s", forbidden, got)
		}
	}
	for _, required := range []string{"http request", "POST", "201"} {
		if !strings.Contains(got, required) {
			t.Fatalf("log missing %q: %s", required, got)
		}
	}
}
