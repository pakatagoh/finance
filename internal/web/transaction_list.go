package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/a-h/templ"
	"github.com/pakatagoh/finance/internal/transactions"
	"github.com/pakatagoh/finance/internal/web/ui"
)

type transactionListQuerier interface {
	Execute(context.Context, transactions.Filter, int) (transactions.Page, error)
}

func TransactionsHandler(store transactionListQuerier) http.Handler {
	return transactionsHandler(store, slog.Default())
}

func TransactionsHandlerWithLogger(store transactionListQuerier, logger *slog.Logger) http.Handler {
	return transactionsHandler(store, logger)
}

func transactionsHandler(store transactionListQuerier, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		filter := transactions.Filter{Bank: r.URL.Query().Get("bank"), Type: r.URL.Query().Get("type"), Category: r.URL.Query().Get("category")}
		p, err := store.Execute(r.Context(), filter, page)
		if err != nil {
			logger.ErrorContext(r.Context(), "load transaction list", "error", "storage failure")
			p = transactions.Page{Filter: filter, Page: page}
			retryValues := url.Values{}
			if filter.Bank != "" {
				retryValues.Set("bank", filter.Bank)
			}
			if filter.Type != "" {
				retryValues.Set("type", filter.Type)
			}
			if filter.Category != "" {
				retryValues.Set("category", filter.Category)
			}
			if rawPage := r.URL.Query().Get("page"); rawPage != "" {
				retryValues.Set("page", rawPage)
			}
			retryURL := "/transactions"
			if query := retryValues.Encode(); query != "" {
				retryURL += "?" + query
			}
			var component templ.Component = ui.TransactionsErrorPage(CSPNonce(r.Context()), p, retryURL)
			if isHTMX(r) {
				component = ui.TransactionsErrorResults(CSPNonce(r.Context()), p, retryURL)
			}
			renderHTML(w, r, http.StatusInternalServerError, component)
			return
		}
		var component templ.Component = ui.TransactionsPage(CSPNonce(r.Context()), p)
		if isHTMX(r) {
			component = ui.TransactionsResults(CSPNonce(r.Context()), p)
		}
		renderHTML(w, r, http.StatusOK, component)
	})
}
