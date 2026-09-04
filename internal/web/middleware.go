// Package web contains the server-rendered UI and HTTP middleware.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

type contextKey string

const nonceKey contextKey = "finance.csp-nonce"

// CSPNonce returns the nonce assigned to the current page response.
func CSPNonce(ctx context.Context) string {
	value, _ := ctx.Value(nonceKey).(string)
	return value
}

func newNonce() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}

// ContentSecurityPolicy adds a fresh nonce and strict CSP to every response.
// Handlers should put that nonce on inline script elements via CSPNonce.
func ContentSecurityPolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := newNonce()
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-"+nonce+"'; style-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nonceKey, nonce)))
	})
}

// BrowserStateChanging protects browser mutation routes against cross-origin
// form submissions. It deliberately does not emit CORS headers.
func BrowserStateChanging(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Origin") != origin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && !strings.EqualFold(site, "same-origin") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BearerAPI authenticates machine-to-machine API requests. Keep this boundary
// separate from BrowserStateChanging: bearer clients do not need browser CSRF
// headers, and UI middleware must never accept bearer credentials as a bypass.
func BearerAPI(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		value := r.Header.Get("Authorization")
		if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) || subtle.ConstantTimeCompare([]byte(value[len(prefix):]), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="finance"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UIHandler is the public composition helper for browser pages.
func UIHandler(origin string, next http.Handler) http.Handler {
	return ContentSecurityPolicy(BrowserStateChanging(origin, next))
}
