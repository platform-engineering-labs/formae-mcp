package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// request describes one agent API call. Every request is built here so that
// endpoint, headers, cancellation and error construction hold on all of them.
// A per-method audit is how one method ends up missing a header.
type request struct {
	Method      string
	Path        string
	Query       url.Values
	Headers     map[string]string
	Body        io.Reader
	ContentType string
}

// do executes a request against the agent and returns the body and status.
func (c *FormaeClient) do(ctx context.Context, r request) ([]byte, int, error) {
	u := c.endpoint + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, r.Method, u, r.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	if r.ContentType != "" {
		req.Header.Set("Content-Type", r.ContentType)
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}
	return body, resp.StatusCode, nil
}
