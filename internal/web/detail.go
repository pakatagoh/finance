package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/pakatagoh/finance/internal/storage"
)

type DetailStore interface {
	storage.TransactionReader
	storage.TransactionEditor
	ActiveCategories(context.Context) ([]storage.Category, error)
}

// normalizeNotes applies the UI's notes contract before persistence.
func normalizeNotes(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if utf8.RuneCountInString(value) > 2000 {
		return "", fmt.Errorf("notes must be 2,000 characters or fewer")
	}
	return value, nil
}

type detailPage struct {
	Transaction storage.Transaction
	Categories  []storage.Category
	Back        string
	Error       string
	Success     string
}

var errInvalidDetailForm = errors.New("invalid detail form")

var detailTemplate = template.Must(template.New("transaction-detail").Funcs(template.FuncMap{"selected": func(id string, current *string) bool { return current != nil && *current == id }}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Transaction · Finance</title><link rel="stylesheet" href="/static/css/app.css"></head>
<body><header class="navbar bg-base-200"><a class="btn btn-ghost text-xl" href="/">Finance</a></header><main id="transaction-detail" class="container mx-auto p-4">
<a href="{{.Back}}">Back</a>
<h1>Transaction detail</h1>
{{if .Success}}<p role="status" aria-live="polite">{{.Success}}</p>{{end}}
{{if .Error}}<p role="alert" aria-live="assertive">{{.Error}}</p>{{end}}
<dl><dt>Date</dt><dd>{{.Transaction.OccurredAt}}</dd><dt>Bank</dt><dd>{{.Transaction.Bank}}</dd><dt>Merchant</dt><dd>{{if .Transaction.Merchant}}{{.Transaction.Merchant}}{{else}}—{{end}}</dd><dt>Amount</dt><dd>{{.Transaction.Currency}} {{.Transaction.AmountMinor}}</dd><dt>Source</dt><dd>{{.Transaction.SourceType}}</dd></dl>
<form method="post" action="/transactions/{{.Transaction.ID}}" hx-action="/transactions/{{.Transaction.ID}}" hx-method="patch" hx-target="#transaction-detail" hx-select="#transaction-detail" hx-swap="outerHTML">
<input type="hidden" name="return_to" value="{{.Back}}">
<label for="category">Category</label><select id="category" name="category_id"><option value="">No category</option>{{if and .Error .Transaction.CategoryID}}<option value="{{.Transaction.CategoryID}}" selected>Submitted category</option>{{end}}{{range .Categories}}<option value="{{.ID}}" {{if selected .ID $.Transaction.CategoryID}}selected{{end}}>{{.Name}}</option>{{end}}</select>
<label for="notes">Notes</label><textarea id="notes" name="notes" maxlength="2000" rows="8">{{if .Transaction.Notes}}{{.Transaction.Notes}}{{end}}</textarea>
<button type="submit">Save</button></form>
</main><script src="/static/js/htmx.min.js"></script><script src="/static/js/hx-live.min.js"></script><script src="/static/js/hx-csp.min.js"></script></body></html>`))

func backURL(raw string) string {
	u, err := url.Parse(raw)
	if raw == "" || err != nil || strings.Contains(raw, "\\") || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || u.IsAbs() || u.Host != "" || strings.HasPrefix(u.Path, "//") {
		return "/transactions"
	}
	return raw
}

func transactionID(r *http.Request) string {
	if id := r.PathValue("uuid"); id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "transactions" {
		return parts[1]
	}
	return ""
}

func detailURL(id, back string) string {
	path := "/transactions/" + url.PathEscape(id)
	if back != "/transactions" {
		path += "?" + url.Values{"return_to": {back}}.Encode()
	}
	return path
}

func isHTMX(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

func renderDetailError(w http.ResponseWriter, status int, msg string, page detailPage) {
	page.Error = msg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = detailTemplate.Execute(w, page)
}

func updateDetail(r *http.Request, store DetailStore, id string) (storage.Transaction, string, error) {
	tx, err := store.GetTransaction(r.Context(), id)
	if err != nil {
		return tx, backURL(r.URL.Query().Get("return_to")), err
	}
	if err := r.ParseForm(); err != nil {
		return tx, backURL(r.URL.Query().Get("return_to")), fmt.Errorf("%w: %v", errInvalidDetailForm, err)
	}
	back := backURL(r.FormValue("return_to"))
	category := strings.TrimSpace(r.FormValue("category_id"))
	var categoryPtr *string
	if category != "" {
		categoryPtr = &category
	}
	submittedNotes := strings.TrimSpace(r.FormValue("notes"))
	if submittedNotes != "" {
		tx.Notes = &submittedNotes
	} else {
		tx.Notes = nil
	}
	tx.CategoryID = categoryPtr
	notes, err := normalizeNotes(submittedNotes)
	if err != nil {
		return tx, back, err
	}
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	saved, err := store.UpdateEnrichment(r.Context(), id, categoryPtr, notesPtr)
	if err != nil {
		return tx, back, err
	}
	return saved, back, nil
}

func TransactionDetailHandler(store DetailStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := transactionID(r)
		if id == "" {
			http.NotFound(w, r)
			return
		}
		back := backURL(r.URL.Query().Get("return_to"))
		if r.Method == http.MethodPatch && !isHTMX(r) {
			http.Error(w, "PATCH requires an HTMX request", http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			tx, back, err := updateDetail(r, store, id)
			if err != nil {
				page := detailPage{Transaction: tx, Categories: categories(r, store), Back: back}
				if errors.Is(err, errInvalidDetailForm) {
					renderDetailError(w, http.StatusBadRequest, "Invalid form", page)
				} else if strings.Contains(err.Error(), "notes must be") {
					renderDetailError(w, http.StatusUnprocessableEntity, err.Error(), page)
				} else if errors.Is(err, storage.ErrTransactionNotFound) {
					http.NotFound(w, r)
				} else if errors.Is(err, storage.ErrInvalidCategory) {
					renderDetailError(w, http.StatusUnprocessableEntity, "Choose an active category or no category", page)
				} else {
					renderDetailError(w, http.StatusInternalServerError, "Unable to save transaction", page)
				}
				return
			}
			if r.Method == http.MethodPatch {
				renderDetail(w, detailPage{Transaction: tx, Categories: categories(r, store), Back: back, Success: "Transaction saved."})
				return
			}
			http.Redirect(w, r, detailURL(id, back), http.StatusSeeOther)
			return
		}
		tx, err := store.GetTransaction(r.Context(), id)
		if errors.Is(err, storage.ErrTransactionNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Unable to load transaction", 500)
			return
		}
		cats, err := store.ActiveCategories(r.Context())
		if err != nil {
			http.Error(w, "Unable to load categories", 500)
			return
		}
		renderDetail(w, detailPage{Transaction: tx, Categories: cats, Back: back})
	})
}

func categories(r *http.Request, store DetailStore) []storage.Category {
	c, _ := store.ActiveCategories(r.Context())
	return c
}
func renderDetail(w http.ResponseWriter, p detailPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = detailTemplate.Execute(w, p)
}
