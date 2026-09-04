package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pakatagoh/finance/internal/storage"
)

type ingestStoreStub struct {
	transaction storage.Transaction
	retry       bool
	err         error
	calls       int
}

func (s *ingestStoreStub) Ingest(context.Context, storage.TransactionInput) (storage.Transaction, bool, error) {
	s.calls++
	return s.transaction, s.retry, s.err
}

func TestCreateTransactionHandlerLogsSanitizedOutcome(t *testing.T) {
	valid := `{"source_mailbox":"user@example.com","gmail_message_id":"message-secret","occurred_at":"2026-09-03T08:15:00Z","timestamp_source":"transaction","source_occurred_text":"03 Sep 2026, 08:15 SGT","bank":"dbs","source_type":"email","kind":"card_purchase","direction":"debit","currency":"SGD","amount_minor":1250,"merchant":"private merchant"}`
	for _, tc := range []struct {
		name, outcome string
		status        int
		store         ingestStoreStub
		body          string
	}{
		{name: "created", outcome: "created", status: http.StatusCreated, store: ingestStoreStub{transaction: storage.Transaction{ID: "transaction-id"}}, body: valid},
		{name: "duplicate", outcome: "duplicate", status: http.StatusOK, store: ingestStoreStub{transaction: storage.Transaction{ID: "transaction-id"}, retry: true}, body: valid},
		{name: "rejected", outcome: "rejected", status: http.StatusUnprocessableEntity, body: `{"source_mailbox":"private@example.com","Authorization":"Bearer token-secret"}`},
		{name: "conflict", outcome: "conflict", status: http.StatusConflict, store: ingestStoreStub{err: storage.ErrSourceConflict}, body: valid},
		{name: "failed", outcome: "failed", status: http.StatusInternalServerError, store: ingestStoreStub{err: errors.New("postgres://user:password@db/finance")}, body: valid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			store := tc.store
			h := CreateTransactionHandlerWithLogger(&store, logger)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer token-secret")
			r.Header.Set("X-Database-URL", "postgres://user:password@db/finance")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want %d body=%s", w.Code, tc.status, w.Body)
			}
			var event map[string]any
			if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
				t.Fatalf("invalid log JSON: %v (%s)", err, logs.String())
			}
			if event["msg"] != "transaction ingestion" || event["outcome"] != tc.outcome || event["status"] != float64(tc.status) {
				t.Fatalf("event=%v", event)
			}
			for _, forbidden := range []string{
				"Authorization", "token-secret", "postgres://", "password", "message-secret",
				"user@example.com", "private merchant", "1250", "03 Sep 2026",
			} {
				if strings.Contains(logs.String(), forbidden) {
					t.Fatalf("log contains forbidden %q: %s", forbidden, logs.String())
				}
			}
		})
	}
}

func TestCreateTransactionHandlerRejectedDoesNotCallStore(t *testing.T) {
	var logs bytes.Buffer
	store := ingestStoreStub{}
	h := CreateTransactionHandlerWithLogger(&store, slog.New(slog.NewJSONHandler(&logs, nil)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/transactions", strings.NewReader(`{"amount_minor":0}`)))
	if store.calls != 0 {
		t.Fatalf("store calls=%d want 0", store.calls)
	}
}
