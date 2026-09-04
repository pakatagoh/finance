package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/web/ui"
)

type transactionListQuerier interface {
	ListTransactions(context.Context, storage.TransactionFilter, int) (storage.TransactionPage, error)
}

func TransactionsHandler(store transactionListQuerier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		p, err := store.ListTransactions(r.Context(), storage.TransactionFilter{Bank: r.URL.Query().Get("bank"), Type: r.URL.Query().Get("type"), Category: r.URL.Query().Get("category")}, page)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var component templ.Component = ui.TransactionsPage(CSPNonce(r.Context()), p)
		if r.Header.Get("HX-Request") == "true" {
			component = ui.TransactionsResults(p)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := component.Render(r.Context(), w); err != nil {
			return
		}
	})
}
