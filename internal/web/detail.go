package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"
	"github.com/pakatagoh/finance/internal/storage"
	"github.com/pakatagoh/finance/internal/web/ui"
)

type detailUseCase interface {
	Load(context.Context, string) (storage.Transaction, []storage.Category, error)
	Save(context.Context, string, string, string) (storage.Transaction, error)
}

type detailPage struct {
	Transaction   storage.Transaction
	Categories    []storage.Category
	Back          string
	Error         string
	Success       string
	CategoryError string
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
	return TransactionDetailHandlerWithLogger(store, slog.Default())
}

func TransactionDetailHandlerWithLogger(store detailUseCase, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
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
				if errors.Is(err, storage.ErrTransactionNotFound) {
					renderDetailNotFound(w, r, back)
					return
				}

				page := detailPage{Transaction: tx, Back: back}
				if errors.Is(err, errInvalidDetailForm) {
					loaded, cats, loadErr := store.Load(r.Context(), id)
					if errors.Is(loadErr, storage.ErrTransactionNotFound) {
						renderDetailNotFound(w, r, back)
						return
					}
					if loaded.ID == "" {
						logger.ErrorContext(r.Context(), "load transaction after invalid form", "error", "storage failure")
						renderDetailLoadError(w, r, id, back)
						return
					}
					page.Transaction = loaded
					page.Categories = cats
					if loadErr != nil {
						page.CategoryError = categoryLoadMessage
					}
					renderDetailError(w, r, http.StatusBadRequest, "Invalid form", page)
					return
				}

				if tx.ID == "" {
					logger.ErrorContext(r.Context(), "load transaction for update", "error", "storage failure")
					renderDetailLoadError(w, r, id, back)
					return
				}

				_, cats, loadErr := store.Load(r.Context(), id)
				page.Categories = cats
				if loadErr != nil {
					page.CategoryError = categoryLoadMessage
				}
				switch {
				case strings.Contains(err.Error(), "notes must be"):
					renderDetailError(w, r, http.StatusUnprocessableEntity, err.Error(), page)
				case errors.Is(err, storage.ErrInvalidCategory):
					renderDetailError(w, r, http.StatusUnprocessableEntity, "Choose an active category or no category", page)
				default:
					logger.ErrorContext(r.Context(), "save transaction detail", "error", "storage failure")
					renderDetailError(w, r, http.StatusInternalServerError, "We couldn’t save your changes. Your changes were not saved. Try again.", page)
				}
				return
			}
			if r.Method == http.MethodPatch {
				reloaded, cats, loadErr := store.Load(r.Context(), id)
				if loadErr != nil && reloaded.ID == "" {
					if errors.Is(loadErr, storage.ErrTransactionNotFound) {
						renderDetailNotFound(w, r, back)
					} else {
						logger.ErrorContext(r.Context(), "load transaction after saving transaction", "error", "storage failure")
						renderDetailLoadError(w, r, id, back)
					}
					return
				}
				if reloaded.ID != "" {
					tx = reloaded
				}
				page := detailPage{Transaction: tx, Categories: cats, Back: back, Success: "Transaction saved."}
				if loadErr != nil {
					logger.ErrorContext(r.Context(), "load categories after saving transaction", "error", "storage failure")
					page.CategoryError = categoryLoadMessage
				}
				renderDetailStatus(w, r, http.StatusOK, page)
				return
			}
			http.Redirect(w, r, detailURL(id, back), http.StatusSeeOther)
			return
		}

		tx, cats, err := store.Load(r.Context(), id)
		if errors.Is(err, storage.ErrTransactionNotFound) {
			renderDetailNotFound(w, r, back)
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "load transaction detail", "error", "storage failure")
			if tx.ID != "" {
				renderDetailStatus(w, r, http.StatusInternalServerError, detailPage{Transaction: tx, Back: back, CategoryError: categoryLoadMessage})
				return
			}
			renderDetailLoadError(w, r, id, back)
			return
		}
		renderDetailStatus(w, r, http.StatusOK, detailPage{Transaction: tx, Categories: cats, Back: back})
	})
}

const categoryLoadMessage = "We couldn’t load categories. Your current category will be kept."

func renderDetailLoadError(w http.ResponseWriter, r *http.Request, id, back string) {
	var component templ.Component = ui.TransactionDetailLoadErrorPage(CSPNonce(r.Context()), id, back)
	if isHTMX(r) {
		component = ui.TransactionDetailLoadError(CSPNonce(r.Context()), id, back)
	}
	renderHTML(w, r, http.StatusInternalServerError, component)
}

func renderDetailNotFound(w http.ResponseWriter, r *http.Request, back string) {
	var component templ.Component = ui.TransactionDetailNotFoundPage(CSPNonce(r.Context()), back)
	if isHTMX(r) {
		component = ui.TransactionDetailNotFound(back)
	}
	renderHTML(w, r, http.StatusNotFound, component)
}

func renderDetailStatus(w http.ResponseWriter, r *http.Request, status int, p detailPage) {
	var component templ.Component = ui.TransactionDetailPage(CSPNonce(r.Context()), p.Transaction, p.Categories, p.Back, p.Error, p.Success, p.CategoryError)
	if isHTMX(r) {
		component = ui.TransactionDetail(CSPNonce(r.Context()), p.Transaction, p.Categories, p.Back, p.Error, p.Success, p.CategoryError)
	}
	renderHTML(w, r, status, component)
}
