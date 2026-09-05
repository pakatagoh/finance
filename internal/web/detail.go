package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/pakatagoh/finance/internal/storage"
)

type detailUseCase interface {
	Load(context.Context, string) (storage.Transaction, []storage.Category, error)
	Save(context.Context, string, string, string) (storage.Transaction, error)
}

type detailPage struct {
	Transaction storage.Transaction
	Categories  []storage.Category
	Back        string
	Error       string
	Success     string
}

var errInvalidDetailForm = errors.New("invalid detail form")

var detailTemplate = template.Must(template.New("transaction-detail").Funcs(template.FuncMap{
	"selected": func(id string, current *string) bool { return current != nil && *current == id },
	"amount":   func(minor int64, currency, direction string) string { return formatAmount(minor, currency, direction) },
	"kindLabel": func(kind string) string {
		switch kind {
		case "credit_card":
			return "Credit card"
		case "debit_card":
			return "Debit card"
		case "paynow":
			return "PayNow"
		case "funds_transfer":
			return "Fund transfer"
		case "incoming_transfer":
			return "Incoming transfer"
		case "reversal":
			return "Reversal"
		default:
			return kind
		}
	},
	"directionLabel": func(direction string) string {
		if direction == "credit" {
			return "Credit"
		}
		if direction == "debit" {
			return "Debit"
		}
		return direction
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Transaction · Finance</title><link rel="stylesheet" href="/static/css/app.css"></head>
<body><header class="navbar bg-base-200"><a class="btn btn-ghost text-xl" href="/">Finance</a></header><main id="transaction-detail" class="container mx-auto p-4">
<a href="{{.Back}}">Back</a>
<h1>Transaction detail</h1>
{{if .Success}}<p role="status" aria-live="polite">{{.Success}}</p>{{end}}
{{if .Error}}<p role="alert" aria-live="assertive">{{.Error}}</p>{{end}}
<dl><dt>Date</dt><dd>{{.Transaction.OccurredAt}}</dd><dt>Bank</dt><dd>{{.Transaction.Bank}}</dd><dt>Type</dt><dd>{{kindLabel .Transaction.Kind}}</dd><dt>Direction</dt><dd>{{directionLabel .Transaction.Direction}}</dd><dt>Merchant</dt><dd>{{if .Transaction.Merchant}}{{.Transaction.Merchant}}{{else}}—{{end}}</dd><dt>Amount</dt><dd>{{amount .Transaction.AmountMinor .Transaction.Currency .Transaction.Direction}}</dd><dt>Source</dt><dd>{{.Transaction.SourceType}}</dd></dl>
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

func updateDetail(r *http.Request, store detailUseCase, id string) (storage.Transaction, string, error) {
	back := backURL(r.URL.Query().Get("return_to"))
	if err := r.ParseForm(); err != nil {
		return storage.Transaction{}, back, fmt.Errorf("%w: %v", errInvalidDetailForm, err)
	}
	back = backURL(r.FormValue("return_to"))
	tx, err := store.Save(r.Context(), id, r.FormValue("category_id"), r.FormValue("notes"))
	return tx, back, err
}

func TransactionDetailHandler(store detailUseCase) http.Handler {
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
				page := detailPage{Transaction: tx, Categories: categories(r, store, id), Back: back}
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
				_, cats, _ := store.Load(r.Context(), id)
				renderDetail(w, detailPage{Transaction: tx, Categories: cats, Back: back, Success: "Transaction saved."})
				return
			}
			http.Redirect(w, r, detailURL(id, back), http.StatusSeeOther)
			return
		}
		tx, cats, err := store.Load(r.Context(), id)
		if errors.Is(err, storage.ErrTransactionNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Unable to load transaction", 500)
			return
		}
		renderDetail(w, detailPage{Transaction: tx, Categories: cats, Back: back})
	})
}

func categories(r *http.Request, store detailUseCase, id string) []storage.Category {
	_, c, _ := store.Load(r.Context(), id)
	return c
}
func renderDetail(w http.ResponseWriter, p detailPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = detailTemplate.Execute(w, p)
}
