package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pakatagoh/finance/internal/storage"
)

type listStoreStub struct {
	page      storage.TransactionPage
	gotFilter storage.TransactionFilter
	gotPage   int
}

func (s *listStoreStub) ListTransactions(_ context.Context, f storage.TransactionFilter, p int) (storage.TransactionPage, error) {
	s.gotFilter = f
	s.gotPage = p
	return s.page, nil
}

func TestTransactionsHandlerEmptyStateFullAndHTMX(t *testing.T) {
	s := &listStoreStub{page: storage.TransactionPage{Filter: storage.TransactionFilter{Bank: "DBS", Type: "card", Category: "Food"}, Page: 2}}
	h := TransactionsHandler(s)
	for _, tc := range []struct {
		name string
		hx   bool
	}{
		{name: "full page"},
		{name: "HTMX fragment", hx: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/transactions?bank=DBS&type=card&category=Food&page=2", nil)
			if tc.hx {
				r.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			body := w.Body.String()
			if w.Code != http.StatusOK || !strings.Contains(body, "No transactions yet") || !strings.Contains(body, "New transactions will appear here after they are received from the bank tracker.") {
				t.Fatalf("empty response: code=%d body=%s", w.Code, body)
			}
			if strings.Contains(body, "No transactions found.") || strings.Contains(body, "<tr>") || strings.Contains(body, "Import") {
				t.Fatalf("empty response contains fake row or CTA: %s", body)
			}
			if tc.hx && strings.Contains(body, "<html") {
				t.Fatalf("HTMX response was not a fragment: %s", body)
			}
			if s.gotFilter != (storage.TransactionFilter{Bank: "DBS", Type: "card", Category: "Food"}) || s.gotPage != 2 {
				t.Fatalf("store args = %+v page %d", s.gotFilter, s.gotPage)
			}
		})
	}
}

func TestTransactionsHandlerFullAndHTMX(t *testing.T) {
	s := &listStoreStub{page: storage.TransactionPage{Total: 26, Page: 2, Items: []storage.TransactionListItem{{ID: "abc", OccurredAt: time.Date(2026, 1, 2, 16, 0, 0, 0, time.UTC), Bank: "DBS", Type: "card", MerchantPayee: "Shop", MaskedSuffix: "1234", Category: "Food", Currency: "SGD", Direction: "debit", AmountMinor: 1250}}, Filter: storage.TransactionFilter{Bank: "DBS"}}}
	h := TransactionsHandler(s)
	r := httptest.NewRequest(http.MethodGet, "/transactions?bank=DBS&type=card&category=food&page=2", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "03 Jan 2026 00:00") || !strings.Contains(body, "-SGD 12.50") || !strings.Contains(body, "page=1") {
		t.Fatalf("full response missing expected content: code=%d body=%s", w.Code, body)
	}
	if s.gotFilter.Bank != "DBS" || s.gotFilter.Type != "card" || s.gotPage != 2 {
		t.Fatalf("store args = %+v page %d", s.gotFilter, s.gotPage)
	}
	r = httptest.NewRequest(http.MethodGet, "/transactions?page=2", nil)
	r.Header.Set("HX-Request", "true")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), "<html") || !strings.Contains(w.Body.String(), "transaction-results") {
		t.Fatalf("HTMX response not fragment: %s", w.Body.String())
	}
}
