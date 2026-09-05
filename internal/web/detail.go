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
<body><header class="navbar border-b border-base-300 bg-base-100"><div class="container mx-auto px-4"><a class="btn btn-ghost px-0 text-xl font-bold" href="/">Finance</a></div></header><main id="transaction-detail" class="container mx-auto max-w-4xl space-y-6 px-4 py-8">
<div class="space-y-2"><p class="text-sm font-medium uppercase tracking-[0.12em] text-base-content/60">Transaction</p><h1 class="text-3xl font-bold tracking-tight">Transaction detail</h1><p class="text-base-content/70">Review the transaction and update its category or notes.</p></div>
{{if .Success}}<div class="alert alert-success" role="status" aria-live="polite"><span>{{.Success}}</span></div>{{end}}
{{if .Error}}<div class="alert alert-error" role="alert" aria-live="assertive"><span>{{.Error}}</span></div>{{end}}
<section class="rounded-box border border-base-300 bg-base-100 shadow-sm" aria-labelledby="transaction-summary-heading">
<div class="flex flex-col gap-3 border-b border-base-300 px-6 py-5 sm:flex-row sm:items-start sm:justify-between"><div><h2 id="transaction-summary-heading" class="text-lg font-semibold">Summary</h2><p class="text-sm text-base-content/60">{{.Transaction.SourceType}}</p></div><p class="text-2xl font-bold tabular-nums sm:text-right">{{amount .Transaction.AmountMinor .Transaction.Currency .Transaction.Direction}}</p></div>
<dl class="grid gap-x-8 gap-y-5 px-6 py-6 sm:grid-cols-2"><div><dt class="text-sm text-base-content/60">Date</dt><dd class="mt-1 font-medium">{{.Transaction.OccurredAt}}</dd></div><div><dt class="text-sm text-base-content/60">Direction</dt><dd class="mt-1 font-medium">{{directionLabel .Transaction.Direction}}</dd></div><div><dt class="text-sm text-base-content/60">Bank</dt><dd class="mt-1 font-medium">{{.Transaction.Bank}}</dd></div><div><dt class="text-sm text-base-content/60">Type</dt><dd class="mt-1 font-medium">{{kindLabel .Transaction.Kind}}</dd></div><div class="sm:col-span-2"><dt class="text-sm text-base-content/60">Merchant</dt><dd class="mt-1 font-medium">{{if .Transaction.Merchant}}{{.Transaction.Merchant}}{{else}}—{{end}}</dd></div></dl>
</section>
<section class="rounded-box border border-base-300 bg-base-100 shadow-sm" aria-labelledby="transaction-edit-heading"><div class="border-b border-base-300 px-6 py-5"><h2 id="transaction-edit-heading" class="text-lg font-semibold">Categorise transaction</h2><p class="text-sm text-base-content/60">Add context to make this transaction easier to find later.</p></div>
<form class="space-y-5 px-6 py-6" method="post" action="/transactions/{{.Transaction.ID}}" hx-action="/transactions/{{.Transaction.ID}}" hx-method="patch" hx-target="#transaction-detail" hx-select="#transaction-detail" hx-swap="outerHTML">
<input type="hidden" name="return_to" value="{{.Back}}">
<div class="form-control"><label class="label" for="category"><span class="label-text font-medium">Category</span></label><select class="select select-bordered w-full" id="category" name="category_id"><option value="">No category</option>{{if and .Error .Transaction.CategoryID}}<option value="{{.Transaction.CategoryID}}" selected>Submitted category</option>{{end}}{{range .Categories}}<option value="{{.ID}}" {{if selected .ID $.Transaction.CategoryID}}selected{{end}}>{{.Name}}</option>{{end}}</select></div>
<div class="form-control"><label class="label" for="notes"><span class="label-text font-medium">Notes</span></label><textarea class="textarea textarea-bordered min-h-32 w-full" id="notes" name="notes" maxlength="2000" rows="6" placeholder="Add a note about this transaction">{{if .Transaction.Notes}}{{.Transaction.Notes}}{{end}}</textarea><div class="label"><span class="label-text-alt text-base-content/60">Optional · up to 2,000 characters</span></div></div>
<div class="flex justify-end border-t border-base-300 pt-5"><button class="btn btn-primary min-w-24" type="submit">Save changes</button></div></form></section>
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
