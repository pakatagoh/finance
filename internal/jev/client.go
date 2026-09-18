// Package jev provides a small provider-neutral client for Jev's typed
// System One HTTP API.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// ErrMalformedResponse indicates that Jev returned an invalid response.
var ErrMalformedResponse = errors.New("malformed Jev response")

// HTTPError reports a non-success HTTP status without retaining response data.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("Jev request failed with status %d", e.StatusCode)
}

// ChoiceQuestion is one named Choice question sent to Jev. Criteria maps the
// canonical option key to its human-readable description.
type ChoiceQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// ChoiceRequest is the complete System One request envelope.
type ChoiceRequest struct {
	Model     string                    `json:"model"`
	State     map[string]any            `json:"state"`
	Questions map[string]ChoiceQuestion `json:"questions"`
}

// ChoiceAnswer is Jev's structured answer for one Choice question.
type ChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

// ChoiceResponse is Jev's response envelope. Usage is retained as structured
// JSON so callers can inspect provider additions without coupling this client.
type ChoiceResponse struct {
	Answers map[string]ChoiceAnswer `json:"answers"`
	Model   string                  `json:"model"`
	Usage   map[string]any          `json:"usage"`
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets the maximum duration for each request.
func WithTimeout(timeout time.Duration) Option { return func(c *Client) { c.timeout = timeout } }

// WithHTTPClient supplies the transport used by Client. Its timeout is not
// relied on; the per-request context timeout still applies.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// Client calls a configurable Jev endpoint.
type Client struct {
	endpoint   string
	token      string
	timeout    time.Duration
	httpClient *http.Client
}

// NewClient constructs a client for endpoint, such as
// https://api.typesafe.ai/v1/systemone.
func NewClient(endpoint, token string, options ...Option) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid Jev endpoint")
	}
	c := &Client{endpoint: endpoint, token: token, timeout: defaultTimeout, httpClient: http.DefaultClient}
	for _, option := range options {
		option(c)
	}
	if c.timeout <= 0 {
		c.timeout = defaultTimeout
	}
	return c, nil
}

// Choice sends a complete typed Choice request and preserves Jev's structured
// response. It does not log credentials, request data, or response bodies.
func (c *Client) Choice(ctx context.Context, request ChoiceRequest) (ChoiceResponse, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return ChoiceResponse{}, fmt.Errorf("encode Jev request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return ChoiceResponse{}, fmt.Errorf("create Jev request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return ChoiceResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return ChoiceResponse{}, &HTTPError{StatusCode: response.StatusCode}
	}
	var result ChoiceResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return ChoiceResponse{}, fmt.Errorf("%w: decode response", ErrMalformedResponse)
	}
	if result.Answers == nil {
		return ChoiceResponse{}, fmt.Errorf("%w: answers missing", ErrMalformedResponse)
	}
	return result, nil
}
