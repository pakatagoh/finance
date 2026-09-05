package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/web/ui"
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

func renderDetailError(w http.ResponseWriter, r *http.Request, status int, msg string, page detailPage) {
	page.Error = msg
	renderDetailStatus(w, r, status, page)
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
					renderDetailError(w, r, http.StatusBadRequest, "Invalid form", page)
				} else if strings.Contains(err.Error(), "notes must be") {
					renderDetailError(w, r, http.StatusUnprocessableEntity, err.Error(), page)
				} else if errors.Is(err, storage.ErrTransactionNotFound) {
					http.NotFound(w, r)
				} else if errors.Is(err, storage.ErrInvalidCategory) {
					renderDetailError(w, r, http.StatusUnprocessableEntity, "Choose an active category or no category", page)
				} else {
					renderDetailError(w, r, http.StatusInternalServerError, "Unable to save transaction", page)
				}
				return
			}
			if r.Method == http.MethodPatch {
				_, cats, _ := store.Load(r.Context(), id)
				renderDetailStatus(w, r, http.StatusOK, detailPage{Transaction: tx, Categories: cats, Back: back, Success: "Transaction saved."})
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
			http.Error(w, "Unable to load transaction", http.StatusInternalServerError)
			return
		}
		renderDetailStatus(w, r, http.StatusOK, detailPage{Transaction: tx, Categories: cats, Back: back})
	})
}

func categories(r *http.Request, store detailUseCase, id string) []storage.Category {
	_, c, _ := store.Load(r.Context(), id)
	return c
}

func renderDetailStatus(w http.ResponseWriter, r *http.Request, status int, p detailPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = ui.TransactionDetailPage(CSPNonce(r.Context()), p.Transaction, p.Categories, p.Back, p.Error, p.Success).Render(r.Context(), w)
}
