package jev

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChoiceSendsTypedSystemOneRequestAndPreservesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Errorf("Authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		want := `{"model":"jev-latest","state":{},"questions":{"category":{"type":"choice","instructions":"Choose a category","criteria":{"food":"Food","other":"None of the above"}}}}`
		if string(body) != want {
			t.Errorf("request body = %s, want %s", body, want)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"answers":{"category":{"choice":"food","probabilities":{"food":0.91,"other":0.09},"confidence":0.88}},"model":"jev-latest","usage":{"input_tokens":12}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Choice(context.Background(), ChoiceRequest{
		Model: "jev-latest", State: map[string]any{}, Questions: map[string]ChoiceQuestion{
			"category": {Type: "choice", Instructions: "Choose a category", Criteria: map[string]string{"food": "Food", "other": "None of the above"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Answers["category"].Choice != "food" || got.Answers["category"].Probabilities["food"] != 0.91 || got.Answers["category"].Confidence != 0.88 || got.Model != "jev-latest" {
		t.Fatalf("response not preserved: %#v", got)
	}
}

func TestChoiceReturnsNon2xxWithoutLeakingTokenOrBody(t *testing.T) {
	const token, body = "secret-token", "private response body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, body, http.StatusUnauthorized) }))
	defer server.Close()
	client, err := NewClient(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Choice(context.Background(), ChoiceRequest{})
	if err == nil || !strings.Contains(err.Error(), "status 401") || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), body) {
		t.Fatalf("safe HTTP error = %v", err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T", err)
	}
}

func TestChoiceReturnsMalformedResponseWithoutBody(t *testing.T) {
	const body = `{"answers":`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Choice(context.Background(), ChoiceRequest{})
	if err == nil || !errors.Is(err, ErrMalformedResponse) || strings.Contains(err.Error(), body) {
		t.Fatalf("malformed error = %v", err)
	}
}

func TestChoiceHonorsConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep the handler bounded so httptest.Server.Close cannot wait forever
		// if a transport does not propagate cancellation to the server promptly.
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token", WithTimeout(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Choice(context.Background(), ChoiceRequest{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}
