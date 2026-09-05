package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/transactions"
)

func parseHTML(t *testing.T, body string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse response HTML: %v", err)
	}
	return doc
}

func TestBackURLRejectsExternal(t *testing.T) {
	if got := backURL("https://evil.example"); got != "/transactions" {
		t.Fatal(got)
	}
	if got := backURL("/transactions?page=2"); got != "/transactions?page=2" {
		t.Fatal(got)
	}
	if got := backURL(`/\\evil.example`); got != "/transactions" {
		t.Fatalf("backURL accepted browser-normalized external path: %q", got)
	}
}

type detailFake struct {
	tx        storage.Transaction
	cats      []storage.Category
	updated   bool
	category  *string
	notes     *string
	updateErr error
}

func (f *detailFake) GetTransaction(context.Context, string) (storage.Transaction, error) {
	return f.tx, nil
}
func (f *detailFake) ActiveCategories(context.Context) ([]storage.Category, error) {
	return f.cats, nil
}
func (f *detailFake) UpdateEnrichment(_ context.Context, _ string, c, n *string) (storage.Transaction, error) {
	f.updated = true
	f.category = c
	f.notes = n
	f.tx.CategoryID = c
	f.tx.Notes = n
	if f.updateErr != nil {
		return storage.Transaction{}, f.updateErr
	}
	return f.tx, nil
}
func TestDetailSaveUsesPRGWithoutHTMX(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank", Currency: "SGD", AmountMinor: 100}, cats: []storage.Category{{ID: "cat", Name: "Food"}}}
	req := httptest.NewRequest(http.MethodPost, "/transactions/abc", strings.NewReader("category_id=cat&notes=hello&return_to=%2Ftransactions%3Fpage%3D2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("PRG status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/transactions/abc?return_to=%2Ftransactions%3Fpage%3D2" {
		t.Fatalf("PRG location = %q", got)
	}
}

func TestDetailSaveSwapsWithHTMX(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank", Currency: "SGD", AmountMinor: 100}, cats: []storage.Category{{ID: "cat", Name: "Food"}}}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("category_id=cat&notes=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	doc := parseHTML(t, rec.Body.String())
	form := doc.Find("main#transaction-detail form")
	if rec.Code != http.StatusOK || doc.Find("main#transaction-detail").Length() != 1 || form.Length() != 1 || form.AttrOr("hx-method", "") != "patch" || form.AttrOr("hx-action", "") != "/transactions/abc" {
		t.Fatalf("HTMX response: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDetailPatchRejectsNonHTMX(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc"}}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("notes=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || f.updated {
		t.Fatalf("non-HTMX PATCH: status=%d updated=%v", rec.Code, f.updated)
	}
}

func TestDetailHTMXErrorKeepsFormAndShowsMessage(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank", Currency: "SGD", AmountMinor: 100}, cats: []storage.Category{{ID: "cat", Name: "Food"}}}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("category_id=cat&notes="+strings.Repeat("x", 2001)+"&return_to=%2Ftransactions%3Fpage%3D2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	body := rec.Body.String()
	doc := parseHTML(t, body)
	if rec.Code != http.StatusUnprocessableEntity || doc.Find("main#transaction-detail").Length() != 1 || !strings.Contains(doc.Find("[role=alert]").Text(), "notes must be 2,000 characters or fewer") || doc.Find("textarea[name=notes]").Length() != 1 || doc.Find("button[type=submit]").Length() != 1 || doc.Find(`input[name=return_to][value="/transactions?page=2"]`).Length() != 1 || f.updated {
		t.Fatalf("HTMX error response: status=%d updated=%v body=%q", rec.Code, f.updated, body)
	}
}

func TestDetailHTMXInvalidCategoryPreservesSubmittedValue(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank"}, cats: []storage.Category{{ID: "cat", Name: "Food"}}, updateErr: storage.ErrInvalidCategory}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("category_id=removed&notes=keep%20this"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	body := rec.Body.String()
	doc := parseHTML(t, body)
	selected := doc.Find(`select[name=category_id] option[value="removed"]`)
	_, selectedAttr := selected.Attr("selected")
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(doc.Find("[role=alert]").Text(), "Choose an active category") || selected.Length() != 1 || !selectedAttr || doc.Find("textarea[name=notes]").Text() != "keep this" {
		t.Fatalf("invalid category response: status=%d body=%q", rec.Code, body)
	}
}

func TestDetailSaveEscapesNotesAndPreservesBack(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank", Currency: "SGD", AmountMinor: 100}, cats: []storage.Category{{ID: "cat", Name: "Food"}}}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("category_id=cat&notes=%20%3Cscript%3Ebad%3C%2Fscript%3E%20&return_to=%2Ftransactions%3Fpage%3D2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !f.updated || f.notes == nil || *f.notes != "<script>bad</script>" {
		t.Fatalf("save: code=%d updated=%v notes=%v", rec.Code, f.updated, f.notes)
	}
	doc := parseHTML(t, rec.Body.String())
	if doc.Find(`a[href="/transactions?page=2"]`).Length() != 0 || doc.Find(`input[name=return_to][value="/transactions?page=2"]`).Length() != 1 || doc.Find("textarea[name=notes]").Text() != "<script>bad</script>" {
		t.Fatal("back state or escaping failed")
	}
	form := doc.Find("main#transaction-detail form")
	if form.Length() != 1 || form.AttrOr("action", "") != "/transactions/abc" || strings.Contains(rec.Body.String(), "/transactions/abc/edit") {
		t.Fatal("save form does not use canonical transaction URL")
	}
	if !strings.Contains(rec.Body.String(), "Transaction saved.") {
		t.Fatal("missing accessible success")
	}
}

func TestDetailDisplaysFormattedAmount(t *testing.T) {
	for _, tc := range []struct {
		name, direction, want string
	}{
		{name: "debit", direction: "debit", want: "-SGD 12.50"},
		{name: "credit", direction: "credit", want: "+SGD 12.50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank", Currency: "SGD", AmountMinor: 1250, Direction: tc.direction}}
			rec := httptest.NewRecorder()
			TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/transactions/abc", nil))
			doc := parseHTML(t, rec.Body.String())
			got := strings.TrimSpace(doc.Find(".tabular-nums").Text())
			if got != tc.want {
				t.Fatalf("detail amount = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetailDisplaysSingaporeFriendlyDateWithoutDirectionField(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{
		ID:         "abc",
		OccurredAt: time.Date(2026, 9, 21, 6, 22, 0, 0, time.UTC),
		Bank:       "Bank",
		Currency:   "SGD",
		Direction:  "debit",
	}}
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/transactions/abc", nil))
	doc := parseHTML(t, rec.Body.String())
	if doc.Find("dd").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.TrimSpace(s.Text()) == "21 September 2026 14:22"
	}).Length() != 1 {
		t.Fatalf("friendly SGT date missing from detail page: %s", rec.Body.String())
	}
	if doc.Find("dt").FilterFunction(func(_ int, s *goquery.Selection) bool { return strings.TrimSpace(s.Text()) == "Direction" }).Length() != 0 {
		t.Fatal("direction should be represented by the signed amount, not a separate field")
	}
}
