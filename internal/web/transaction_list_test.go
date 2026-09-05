package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pakatagoh/finance/internal/transactions"
)

type listStoreStub struct {
	page      transactions.Page
	gotFilter transactions.Filter
	gotPage   int
}

func (s *listStoreStub) Execute(_ context.Context, f transactions.Filter, p int) (transactions.Page, error) {
	s.gotFilter = f
	s.gotPage = p
	return s.page, nil
}

func TestTransactionsHandlerEmptyStateFullAndHTMX(t *testing.T) {
	s := &listStoreStub{page: transactions.Page{Filter: transactions.Filter{Bank: "DBS", Type: "card", Category: "Food"}, Page: 2}}
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
			if w.Code != http.StatusOK || !strings.Contains(body, "No matching transactions") || !strings.Contains(body, "Try adjusting your filters to see more transactions.") {
				t.Fatalf("empty response: code=%d body=%s", w.Code, body)
			}
			if strings.Contains(body, "No transactions found.") || strings.Contains(body, "<tr>") || strings.Contains(body, "Import") {
				t.Fatalf("empty response contains fake row or CTA: %s", body)
			}
			if tc.hx && strings.Contains(body, "<html") {
				t.Fatalf("HTMX response was not a fragment: %s", body)
			}
			if s.gotFilter != (transactions.Filter{Bank: "DBS", Type: "card", Category: "Food"}) || s.gotPage != 2 {
				t.Fatalf("store args = %+v page %d", s.gotFilter, s.gotPage)
			}
		})
	}
}

func TestTransactionsHandlerFullAndHTMX(t *testing.T) {
	s := &listStoreStub{page: transactions.Page{Total: 26, Page: 2, Items: []transactions.ListItem{{ID: "abc", OccurredAt: time.Date(2026, 1, 2, 16, 0, 0, 0, time.UTC), Bank: "DBS", Type: "card", MerchantPayee: "Shop", MaskedSuffix: "1234", Category: "Food", Currency: "SGD", Direction: "debit", AmountMinor: 1250}}, Filter: transactions.Filter{Bank: "DBS"}}}
	h := TransactionsHandler(s)
	r := httptest.NewRequest(http.MethodGet, "/transactions?bank=DBS&type=card&category=food&page=2", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "03 January 2026 00:00") || !strings.Contains(body, "-SGD 12.50") || !strings.Contains(body, "page=1") {
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

func TestTransactionsHandlerColumnsAndDetailAction(t *testing.T) {
	s := &listStoreStub{page: transactions.Page{Total: 1, Items: []transactions.ListItem{{ID: "abc", MerchantPayee: "Shop", Category: "Food", Bank: "DBS", Type: "card", Currency: "SGD", Direction: "debit", AmountMinor: 1250}}}}
	w := httptest.NewRecorder()
	TransactionsHandler(s).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/transactions", nil))
	doc := parseHTML(t, w.Body.String())
	headers := doc.Find("table thead th")
	for i, want := range []string{"Date (SGT)", "Amount", "Merchant / Payee", "Category", "Bank", "Type", "Account", "Actions"} {
		if got := strings.TrimSpace(headers.Eq(i).Text()); got != want {
			t.Fatalf("header %d = %q, want %q", i+1, got, want)
		}
	}
	row := doc.Find("table tbody tr").First()
	if row.Find(`td:eq(2) a`).Length() != 0 || doc.Find(`table tbody td:last-child a`).Text() != "View" || doc.Find(`table tbody td:last-child a`).AttrOr("href", "") != "/transactions/abc" {
		t.Fatalf("merchant should not be the detail link; row=%s body=%s", row.Text(), w.Body.String())
	}
}
