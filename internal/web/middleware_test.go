package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func endpoint() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func TestBrowserStateChangingRequiresExactOrigin(t *testing.T) {
	h := BrowserStateChanging("http://localhost:8080", endpoint())
	cases := []struct {
		name, origin string
		want         int
	}{
		{"exact", "http://localhost:8080", http.StatusNoContent},
		{"subdomain", "http://localhost:8080.evil.test", http.StatusForbidden},
		{"scheme", "https://localhost:8080", http.StatusForbidden},
		{"missing", "", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/transactions", nil)
			r.Header.Set("Origin", tc.origin)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
		})
	}
}

func TestBrowserStateChangingRejectsCrossSiteFetchMetadata(t *testing.T) {
	h := BrowserStateChanging("http://localhost:8080", endpoint())
	r := httptest.NewRequest(http.MethodPost, "/transactions", nil)
	r.Header.Set("Origin", "http://localhost:8080")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestBearerAPIIsSeparateBoundary(t *testing.T) {
	h := BearerAPI("token", endpoint())
	for _, auth := range []string{"", "Bearer wrong", "Basic token"} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", nil)
		r.Header.Set("Authorization", auth)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("auth %q: status = %d", auth, rr.Code)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", nil)
	r.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("valid token status = %d", rr.Code)
	}
}

func TestContentSecurityPolicyUsesFreshNonce(t *testing.T) {
	h := ContentSecurityPolicy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := CSPNonce(r.Context())
		if nonce == "" {
			t.Error("missing context nonce")
		}
		w.WriteHeader(http.StatusOK)
	}))
	var values []string
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		csp := rr.Header().Get("Content-Security-Policy")
		marker := "'nonce-"
		start := strings.Index(csp, marker)
		if start < 0 {
			t.Fatal("missing CSP nonce")
		}
		start += len(marker)
		end := strings.IndexByte(csp[start:], '\'')
		values = append(values, csp[start:start+end])
	}
	if values[0] == values[1] {
		t.Fatal("CSP nonce was reused")
	}
}

func TestBrowserMiddlewareDoesNotEmitCORS(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	BrowserStateChanging("http://localhost:8080", endpoint()).ServeHTTP(rr, r)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS header %q", got)
	}
}
