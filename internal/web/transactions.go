package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pakatagoh/finance/internal/storage"
)

type IngestStore interface {
	Ingest(context.Context, storage.TransactionInput) (storage.Transaction, bool, error)
}

// CreateTransactionHandler is the production handler; it enforces the frozen wire shape.
func CreateTransactionHandler(store IngestStore) http.Handler {
	return CreateTransactionHandlerWithLogger(store, nil)
}

// CreateTransactionHandlerWithLogger records only the ingestion outcome and
// HTTP status. In particular, it never logs the request, source identity,
// transaction fields, or storage errors (which may contain credentials).
func CreateTransactionHandlerWithLogger(store IngestStore, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record := func(outcome string, status int) {
			logger.Info("transaction ingestion", "outcome", outcome, "status", status)
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/api/v1/transactions" && r.URL.Path != "/transactions" {
			http.NotFound(w, r)
			return
		}
		var req createRequest
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		d.DisallowUnknownFields()
		if err := d.Decode(&req); err != nil {
			record("rejected", http.StatusUnprocessableEntity)
			writeError(w, 422, "unprocessable_entity", "invalid request")
			return
		}
		in, ok := req.input()
		if !ok {
			record("rejected", http.StatusUnprocessableEntity)
			writeError(w, 422, "unprocessable_entity", "invalid request")
			return
		}
		out, retry, err := store.Ingest(r.Context(), in)
		if err != nil {
			if errors.Is(err, storage.ErrSourceConflict) {
				record("conflict", http.StatusConflict)
				writeError(w, 409, "conflict", "source identity already exists with different values")
				return
			}
			record("failed", http.StatusInternalServerError)
			writeError(w, 500, "internal", "internal server error")
			return
		}
		status := http.StatusCreated
		outcome := "created"
		if retry {
			status = http.StatusOK
			outcome = "duplicate"
		}
		record(outcome, status)
		writeJSON(w, status, out)
	})
}

type createRequest struct {
	SourceMailbox      string  `json:"source_mailbox"`
	GmailMessageID     string  `json:"gmail_message_id"`
	OccurredAt         string  `json:"occurred_at"`
	TimestampSource    string  `json:"timestamp_source"`
	SourceOccurredText string  `json:"source_occurred_text"`
	Bank               string  `json:"bank"`
	SourceType         string  `json:"source_type"`
	Kind               string  `json:"kind"`
	Direction          string  `json:"direction"`
	Currency           string  `json:"currency"`
	AmountMinor        *int64  `json:"amount_minor"`
	CardSuffix         *string `json:"card_suffix"`
	FromAccountSuffix  *string `json:"from_account_suffix"`
	Payee              *string `json:"payee"`
	Merchant           *string `json:"merchant"`
}

func (r createRequest) input() (storage.TransactionInput, bool) {
	if r.SourceMailbox == "" || r.GmailMessageID == "" || r.OccurredAt == "" || r.TimestampSource == "" || r.SourceOccurredText == "" || r.Bank == "" || r.SourceType == "" || r.Kind == "" || r.Direction == "" || r.Currency == "" || r.AmountMinor == nil || *r.AmountMinor <= 0 {
		return storage.TransactionInput{}, false
	}
	if !strings.HasSuffix(r.OccurredAt, "Z") || len(r.OccurredAt) < 2 {
		return storage.TransactionInput{}, false
	}
	tm, e := time.Parse(time.RFC3339Nano, r.OccurredAt)
	if e != nil || !tm.UTC().Equal(tm) {
		return storage.TransactionInput{}, false
	}
	if len(r.Currency) != 3 || r.Currency != strings.ToUpper(r.Currency) {
		return storage.TransactionInput{}, false
	}
	if !oneOf(r.TimestampSource, "transaction", "email_received", "inferred") || !oneOf(r.Kind, "credit_card", "debit_card", "paynow", "funds_transfer", "incoming_transfer", "reversal") || !oneOf(r.Direction, "debit", "credit") {
		return storage.TransactionInput{}, false
	}
	return storage.TransactionInput{SourceMailbox: r.SourceMailbox, GmailMessageID: r.GmailMessageID, OccurredAt: tm.UTC(), TimestampSource: r.TimestampSource, SourceOccurredText: r.SourceOccurredText, Bank: r.Bank, SourceType: r.SourceType, Kind: r.Kind, Direction: r.Direction, Currency: r.Currency, AmountMinor: *r.AmountMinor, CardSuffix: r.CardSuffix, FromAccountSuffix: r.FromAccountSuffix, Payee: r.Payee, Merchant: r.Merchant}, true
}
func oneOf(s string, vals ...string) bool {
	for _, v := range vals {
		if s == v {
			return true
		}
	}
	return false
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
