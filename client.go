package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiEndpoint = "https://api.linear.app/graphql"
const maxResponseBytes = 16 << 20

type gqlError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}
type apiError struct {
	HTTPStatus int            `json:"httpStatus"`
	Errors     []gqlError     `json:"errors,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	RetryAfter string         `json:"retryAfter,omitempty"`
}

func (e *apiError) Error() string {
	parts := []string{}
	for _, g := range e.Errors {
		s := g.Message
		if code, ok := g.Extensions["code"].(string); ok {
			s += " [" + code + "]"
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		parts = append(parts, http.StatusText(e.HTTPStatus))
	}
	if e.RetryAfter != "" {
		parts = append(parts, "retry-after="+e.RetryAfter)
	}
	return fmt.Sprintf("Linear API HTTP %d: %s", e.HTTPStatus, strings.Join(parts, "; "))
}

type apiClient struct {
	endpoint, token string
	http            *http.Client
}

func newAPIClient(endpoint, token string, timeout time.Duration) *apiClient {
	return &apiClient{endpoint: endpoint, token: token, http: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *apiClient) do(ctx context.Context, query string, vars map[string]any) (map[string]any, error) {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.token)
	req.Header.Set("User-Agent", "linear-cli/"+version)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Linear request failed (not retried; verify whether a mutation applied before repeating): %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Linear response (not retried): %w", err)
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("Linear response exceeds 16 MiB; reduce --first or requested fields")
	}
	// An API error can echo request data. Redact credentials before parsing/output.
	for _, secret := range []string{c.token, strings.TrimPrefix(c.token, "Bearer ")} {
		if secret != "" {
			raw = bytes.ReplaceAll(raw, []byte(secret), []byte("[REDACTED]"))
		}
	}
	var result struct {
		Data   map[string]any `json:"data"`
		Errors []gqlError     `json:"errors"`
	}
	parseErr := json.Unmarshal(raw, &result)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || len(result.Errors) > 0 {
		return nil, &apiError{resp.StatusCode, result.Errors, result.Data, resp.Header.Get("Retry-After")}
	}
	if parseErr != nil {
		return nil, fmt.Errorf("Linear returned invalid JSON")
	}
	if result.Data == nil {
		return nil, fmt.Errorf("Linear response has no data")
	}
	return result.Data, nil
}
