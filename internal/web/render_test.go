package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestRenderHTMLBuffersBeforeCommittingResponse(t *testing.T) {
	component := templ.ComponentFunc(func(context.Context, io.Writer) error {
		return errors.New("template failed")
	})
	rec := httptest.NewRecorder()
	renderHTML(rec, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusUnprocessableEntity, component)
	if rec.Code != http.StatusInternalServerError || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") || strings.Contains(rec.Body.String(), "template failed") || !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("render fallback: status=%d type=%q body=%q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestRenderHTMLHTMXFallbackKeepsDetailTarget(t *testing.T) {
	component := templ.ComponentFunc(func(context.Context, io.Writer) error {
		return errors.New("template failed")
	})
	req := httptest.NewRequest(http.MethodGet, "/transactions/abc", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	renderHTML(rec, req, http.StatusInternalServerError, component)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `id="transaction-detail"`) || !strings.Contains(rec.Body.String(), `role="alert"`) {
		t.Fatalf("HTMX detail fallback lost swap target: %q", rec.Body.String())
	}
}
