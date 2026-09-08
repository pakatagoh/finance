package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"
)

const fallbackHTML = "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>Error · Finance</title></head><body><main><h1>Something went wrong</h1><p>Please try again.</p><p><a href=\"/transactions\">Return to transactions</a></p></main></body></html>"

func renderFallbackHTML(r *http.Request) []byte {
	if isHTMX(r) {
		switch {
		case transactionID(r) != "":
			return []byte(`<main id="transaction-detail" role="alert"><h1>Something went wrong</h1><p>Please try again.</p><a href="/transactions">Return to transactions</a></main>`)
		case strings.HasPrefix(r.URL.Path, "/transactions"):
			return []byte(`<section id="transaction-results" role="alert"><p>Something went wrong. Please try again.</p><a href="/transactions">Retry</a></section>`)
		}
	}
	return []byte(fallbackHTML)
}

func renderHTML(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	var body bytes.Buffer
	if err := component.Render(r.Context(), &body); err != nil {
		slog.Default().ErrorContext(r.Context(), "render HTML response", "error", "template rendering failure")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(renderFallbackHTML(r))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
