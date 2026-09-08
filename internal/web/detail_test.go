package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
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

func responseNonce(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	match := regexp.MustCompile(`nonce-([^']+)`).FindStringSubmatch(rec.Header().Get("Content-Security-Policy"))
	if len(match) != 2 || match[1] == "" {
		t.Fatalf("response CSP nonce missing: %q", rec.Header().Get("Content-Security-Policy"))
	}
	return match[1]
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
	getErr    error
	catsErr   error
}

func (f *detailFake) GetTransaction(context.Context, string) (storage.Transaction, error) {
	return f.tx, f.getErr
}
func (f *detailFake) ActiveCategories(context.Context) ([]storage.Category, error) {
	return f.cats, f.catsErr
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

func TestDetailLoadFailureRendersFullPageOrHTMXFragment(t *testing.T) {
	for _, tc := range []struct {
		name string
		hx   bool
	}{{name: "full page"}, {name: "HTMX fragment", hx: true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &detailFake{getErr: errors.New("connection failed")}
			req := httptest.NewRequest(http.MethodGet, "/transactions/abc?return_to=%2Ftransactions%3Fbank%3DDBS", nil)
			if tc.hx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
			doc := parseHTML(t, rec.Body.String())
			if rec.Code != http.StatusInternalServerError || doc.Find("main#transaction-detail").Length() != 1 || !strings.Contains(doc.Find("[role=alert]").Text(), "We couldn’t load this transaction") {
				t.Fatalf("detail load failure: status=%d body=%s", rec.Code, rec.Body.String())
			}
			if doc.Find(`a[href="/transactions/abc?return_to=%2Ftransactions%3Fbank%3DDBS"]`).Length() != 1 || doc.Find(`a[href="/transactions?bank=DBS"]`).Length() != 1 {
				t.Fatalf("retry/return actions missing: %s", rec.Body.String())
			}
			if tc.hx && strings.Contains(rec.Body.String(), "<html") {
				t.Fatalf("HTMX response contains shell")
			}
			if !tc.hx && doc.Find("html").Length() != 1 {
				t.Fatalf("normal response missing shell")
			}
		})
	}
}

func TestDetailHandlerDoesNotLogRawStorageError(t *testing.T) {
	var logs bytes.Buffer
	secret := "postgres://user:password@db/finance"
	h := TransactionDetailHandlerWithLogger(
		transactions.NewDetailUseCase(&detailFake{getErr: errors.New(secret)}),
		slog.New(slog.NewJSONHandler(&logs, nil)),
	)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/transactions/abc", nil))
	if strings.Contains(logs.String(), secret) || !strings.Contains(logs.String(), "storage failure") {
		t.Fatalf("unsafe or missing detail error log: %s", logs.String())
	}
}

func TestDetailLoadErrorHTMXRetryCarriesResponseNonce(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/transactions/abc", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	ContentSecurityPolicy(TransactionDetailHandler(transactions.NewDetailUseCase(&detailFake{getErr: errors.New("unavailable")}))).ServeHTTP(rec, req)
	nonce := responseNonce(t, rec)
	doc := parseHTML(t, rec.Body.String())
	if doc.Find(`a[hx-get][hx-nonce="`+nonce+`"]`).Length() != 1 {
		t.Fatalf("detail error retry missing response nonce: %s", rec.Body.String())
	}
}

func TestDetailMissingRendersDistinctFullPageOrHTMXFragment(t *testing.T) {
	for _, tc := range []struct {
		name string
		hx   bool
	}{{name: "full page"}, {name: "HTMX fragment", hx: true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &detailFake{getErr: storage.ErrTransactionNotFound}
			req := httptest.NewRequest(http.MethodGet, "/transactions/missing?return_to=%2Ftransactions%3Fbank%3DDBS", nil)
			if tc.hx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
			doc := parseHTML(t, rec.Body.String())
			if rec.Code != http.StatusNotFound || doc.Find("main#transaction-detail").Length() != 1 || !strings.Contains(doc.Find("[role=alert]").Text(), "Transaction not found") || strings.Contains(rec.Body.String(), "couldn’t load this transaction") {
				t.Fatalf("missing response: status=%d body=%s", rec.Code, rec.Body.String())
			}
			if doc.Find(`a[href="/transactions?bank=DBS"]`).Length() != 1 {
				t.Fatalf("missing response lost return navigation: %s", rec.Body.String())
			}
			if tc.hx && strings.Contains(rec.Body.String(), "<html") {
				t.Fatal("HTMX not-found response contains shell")
			}
			if !tc.hx && doc.Find("html").Length() != 1 {
				t.Fatal("native not-found response missing shell")
			}
		})
	}
}

func TestDetailCategoryLoadFailureKeepsSummaryAndProtectsCategory(t *testing.T) {
	category := "existing"
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "DBS", Currency: "SGD", AmountMinor: 1250, CategoryID: &category}, catsErr: errors.New("categories unavailable")}
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/transactions/abc", nil))
	doc := parseHTML(t, rec.Body.String())
	if rec.Code != http.StatusInternalServerError || !strings.Contains(doc.Find(`[aria-labelledby="transaction-summary-heading"]`).Text(), "DBS") || !strings.Contains(doc.Find(`[aria-labelledby="transaction-edit-heading"] [role=alert]`).Text(), "We couldn’t load categories") {
		t.Fatalf("category failure response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	_, categoryDisabled := doc.Find(`select[name=category_id]`).Attr("disabled")
	selected := doc.Find(`select[name=category_id] option[value="existing"]`)
	_, selectedAttr := selected.Attr("selected")
	if !categoryDisabled || doc.Find(`input[type=hidden][name=category_id][value="existing"]`).Length() != 1 || selected.Length() != 1 || !selectedAttr || !strings.Contains(selected.Text(), "temporarily unavailable") {
		t.Fatalf("existing category not protected: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "We couldn’t load this transaction") {
		t.Fatal("category failure became whole-page failure")
	}
}

func TestDetailCategoryFailureFormCanSaveNotesWithoutClearingCategory(t *testing.T) {
	category := "existing"
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "DBS", CategoryID: &category}, catsErr: errors.New("categories unavailable")}
	req := httptest.NewRequest(http.MethodPost, "/transactions/abc", strings.NewReader("category_id=existing&notes=new+note"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || f.category == nil || *f.category != "existing" || f.notes == nil || *f.notes != "new note" {
		t.Fatalf("protected category save: status=%d category=%v notes=%v", rec.Code, f.category, f.notes)
	}
}

func TestDetailHTMXSaveSuccessReportsCategoryRefreshFailureAccurately(t *testing.T) {
	category := "existing"
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "DBS", CategoryID: &category}, catsErr: errors.New("categories unavailable")}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("category_id=existing&notes=new+note"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	doc := parseHTML(t, rec.Body.String())
	if rec.Code != http.StatusOK || !strings.Contains(doc.Find(`[role=status]`).Text(), "Transaction saved") || !strings.Contains(doc.Find(`[aria-labelledby="transaction-edit-heading"] [role=alert]`).Text(), "We couldn’t load categories") {
		t.Fatalf("saved transaction/category refresh response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "changes were not saved") {
		t.Fatal("successful update was reported as not saved")
	}
	_, categoryDisabled := doc.Find(`select[name=category_id]`).Attr("disabled")
	if !categoryDisabled || doc.Find(`input[type=hidden][name=category_id][value="existing"]`).Length() != 1 {
		t.Fatalf("saved category not protected after refresh failure: %s", rec.Body.String())
	}
}

func TestDetailUnexpectedSaveFailurePreservesSubmittedForm(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		hx           bool
	}{{name: "native POST", method: http.MethodPost}, {name: "HTMX PATCH", method: http.MethodPatch, hx: true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "DBS", Currency: "SGD", AmountMinor: 1250}, cats: []storage.Category{{ID: "cat", Name: "Food"}}, updateErr: errors.New("write failed")}
			req := httptest.NewRequest(tc.method, "/transactions/abc", strings.NewReader("category_id=cat&notes=keep+this&return_to=%2Ftransactions%3Fpage%3D2"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.hx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
			doc := parseHTML(t, rec.Body.String())
			selected := doc.Find(`option[value="cat"]`)
			_, selectedAttr := selected.Attr("selected")
			if rec.Code != http.StatusInternalServerError || doc.Find("main#transaction-detail").Length() != 1 || !strings.Contains(doc.Find("[role=alert]").Text(), "We couldn’t save your changes. Your changes were not saved. Try again.") || !strings.Contains(doc.Find(`[aria-labelledby="transaction-summary-heading"]`).Text(), "DBS") || selected.Length() != 1 || !selectedAttr || doc.Find("textarea[name=notes]").Text() != "keep this" || doc.Find(`input[name=return_to][value="/transactions?page=2"]`).Length() != 1 {
				t.Fatalf("save failure: status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.hx && strings.Contains(rec.Body.String(), "<html") {
				t.Fatal("HTMX save failure contains shell")
			}
			if !tc.hx && doc.Find("html").Length() != 1 {
				t.Fatal("native save failure missing shell")
			}
		})
	}
}

func TestDetailUpdateLoadFailureUsesLoadErrorNotEmptyForm(t *testing.T) {
	f := &detailFake{getErr: errors.New("load failed")}
	req := httptest.NewRequest(http.MethodPost, "/transactions/abc", strings.NewReader("category_id=cat&notes=keep"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	TransactionDetailHandler(transactions.NewDetailUseCase(f)).ServeHTTP(rec, req)
	doc := parseHTML(t, rec.Body.String())
	if rec.Code != http.StatusInternalServerError || !strings.Contains(doc.Find("[role=alert]").Text(), "We couldn’t load this transaction") || doc.Find("form").Length() != 0 || strings.Contains(rec.Body.String(), "Unable to save transaction") {
		t.Fatalf("update load failure: status=%d body=%s", rec.Code, rec.Body.String())
	}
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

func TestDetailPageCarriesCSPNonceIntoScriptsAndHTMXForm(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank"}}
	rec := httptest.NewRecorder()
	ContentSecurityPolicy(TransactionDetailHandler(transactions.NewDetailUseCase(f))).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/transactions/abc", nil))
	nonce := responseNonce(t, rec)
	doc := parseHTML(t, rec.Body.String())
	if doc.Find(`script[nonce="`+nonce+`"]`).Length() != 3 || doc.Find(`form[hx-nonce="`+nonce+`"]`).Length() != 1 {
		t.Fatalf("detail nonce propagation failed: nonce=%q body=%s", nonce, rec.Body.String())
	}
}

func TestDetailHTMXResponseCarriesFreshNonce(t *testing.T) {
	f := &detailFake{tx: storage.Transaction{ID: "abc", Bank: "Bank"}, cats: []storage.Category{{ID: "cat", Name: "Food"}}}
	req := httptest.NewRequest(http.MethodPatch, "/transactions/abc", strings.NewReader("category_id=cat&notes=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	ContentSecurityPolicy(TransactionDetailHandler(transactions.NewDetailUseCase(f))).ServeHTTP(rec, req)
	nonce := responseNonce(t, rec)
	doc := parseHTML(t, rec.Body.String())
	if doc.Find(`form[hx-nonce="`+nonce+`"]`).Length() != 1 {
		t.Fatalf("HTMX response nonce propagation failed: nonce=%q body=%s", nonce, rec.Body.String())
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
