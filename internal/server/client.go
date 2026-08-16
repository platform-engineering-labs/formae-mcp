package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

// FormaeClient is a lightweight HTTP client for the formae agent REST API.
//
// There is no endpoint field: where a request goes and what authenticates it
// are one decision, held by the routing, so neither can be paired with the
// other arm's.
type FormaeClient struct {
	route      routing
	httpClient *http.Client
}

// NewFormaeClient builds a classic client for an endpoint that already carries
// its port.
func NewFormaeClient(endpoint string) *FormaeClient {
	return &FormaeClient{
		route:      classicRoute{base: endpoint},
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// newClientFromCtx builds a client for a resolved connection.
//
// The hosted connection is re-validated here rather than trusted from the CLI.
// The MCP exposes tools that write profiles, so a model fed hostile input can
// author a hosted profile naming the real issuer with an attacker's endpoint;
// checking again at the last point before a credential reaches a header is what
// makes that a refusal rather than an exfiltration.
//
// A classic connection with no port of its own carries one in its URL (a forced
// endpoint), so it is used as-is.
func newClientFromCtx(ec execctx.Context, refresh refresher) (*FormaeClient, error) {
	switch conn := ec.Conn.(type) {
	case config.Classic:
		endpoint := conn.URL
		if conn.Port != 0 {
			endpoint = fmt.Sprintf("%s:%d", conn.URL, conn.Port)
		}
		return NewFormaeClient(endpoint), nil

	case config.Hosted:
		if err := config.ValidateHosted(conn); err != nil {
			return nil, fmt.Errorf("profile %q resolved an unusable hosted connection: %w",
				ec.ProfileName, err)
		}
		if ec.Credential.IsZero() {
			return nil, fmt.Errorf(
				"profile %q targets hosted formae but resolved no credential, so it cannot be authenticated",
				ec.ProfileName)
		}
		return &FormaeClient{
			route: &hostedRoute{
				endpoint:     conn.Endpoint,
				installation: conn.Installation,
				credential:   ec.Credential,
				refreshFn:    refresh,
			},
			httpClient: &http.Client{
				Timeout: 30 * time.Second,
				// Refused rather than followed; see refuseRedirects.
				CheckRedirect: refuseRedirects,
			},
		}, nil

	default:
		return nil, fmt.Errorf("profile %q resolved no usable connection", ec.ProfileName)
	}
}

func (c *FormaeClient) get(ctx context.Context, path string, query url.Values, retry retryPolicy) ([]byte, int, error) {
	return c.do(ctx, request{Method: http.MethodGet, Path: path, Query: query}, retry)
}

func (c *FormaeClient) post(ctx context.Context, path string, query url.Values, retry retryPolicy) ([]byte, int, error) {
	return c.do(ctx, request{
		Method:      http.MethodPost,
		Path:        path,
		Query:       query,
		ContentType: "application/json",
	}, retry)
}

// ListResources queries the agent for resources matching the given query string.
func (c *FormaeClient) ListResources(ctx context.Context, query string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}

	body, status, err := c.get(ctx, "/api/v1/resources", q, retryOnce)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return c.route.collectionMiss(json.RawMessage("[]"))
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListStacks retrieves all stacks from the agent.
func (c *FormaeClient) ListStacks(ctx context.Context) (json.RawMessage, error) {
	body, status, err := c.get(ctx, "/api/v1/stacks", nil, retryOnce)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return c.route.collectionMiss(json.RawMessage("[]"))
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListPolicies retrieves all standalone policies from the agent.
func (c *FormaeClient) ListPolicies(ctx context.Context) (json.RawMessage, error) {
	body, status, err := c.get(ctx, "/api/v1/policies", nil, retryOnce)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return c.route.collectionMiss(json.RawMessage("[]"))
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListTargets queries the agent for targets matching the given query string.
func (c *FormaeClient) ListTargets(ctx context.Context, query string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}

	body, status, err := c.get(ctx, "/api/v1/targets", q, retryOnce)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return c.route.collectionMiss(json.RawMessage("[]"))
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// GetCommandStatus retrieves the status of a specific command.
func (c *FormaeClient) GetCommandStatus(ctx context.Context, commandID string, clientID string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("id", commandID)

	body, status, err := c.do(ctx, request{
		Method:  http.MethodGet,
		Path:    "/api/v1/commands/status",
		Query:   q,
		Headers: map[string]string{"Client-ID": clientID},
	}, retryOnce)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("command %s not found", commandID)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListCommands retrieves command statuses matching an optional query.
func (c *FormaeClient) ListCommands(ctx context.Context, query string, maxResults string, clientID string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}
	if maxResults != "" {
		q.Set("max_results", maxResults)
	}

	body, status, err := c.do(ctx, request{
		Method:  http.MethodGet,
		Path:    "/api/v1/commands/status",
		Query:   q,
		Headers: map[string]string{"Client-ID": clientID},
	}, retryOnce)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return c.route.collectionMiss(json.RawMessage(`{"Commands":[]}`))
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// GetAgentStats retrieves agent statistics.
func (c *FormaeClient) GetAgentStats(ctx context.Context) (json.RawMessage, error) {
	body, status, err := c.get(ctx, "/api/v1/stats", nil, retryOnce)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// CheckHealth checks if the agent is healthy.
func (c *FormaeClient) CheckHealth(ctx context.Context) error {
	_, status, err := c.get(ctx, "/api/v1/health", nil, retryOnce)
	if err != nil {
		return fmt.Errorf("agent is not reachable: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("agent returned unhealthy status: %d", status)
	}

	return nil
}

// SubmitCommand submits a forma command (apply/destroy) to the agent.
func (c *FormaeClient) SubmitCommand(ctx context.Context, command string, mode string, simulate bool, force bool, formaJSON []byte, clientID string) (json.RawMessage, error) {
	fields := map[string]string{
		"command":  command,
		"simulate": fmt.Sprintf("%t", simulate),
	}
	if mode != "" {
		fields["mode"] = mode
	}
	if force {
		fields["force"] = "true"
	}

	var fileContent []byte
	var fileField, fileName string
	if formaJSON != nil {
		fileField = "file"
		fileName = "forma.json"
		fileContent = formaJSON
	}

	body, status, err := c.postMultipartWithHeaders(ctx, "/api/v1/commands", nil, fields, fileField, fileName, fileContent, map[string]string{"Client-ID": clientID})
	if err != nil {
		return nil, err
	}
	if !isCommandStatusOK(status, simulate) {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// DestroyByQuery submits a destroy-by-query command to the agent.
func (c *FormaeClient) DestroyByQuery(ctx context.Context, query string, simulate bool, clientID string) (json.RawMessage, error) {
	fields := map[string]string{
		"command":  "destroy",
		"query":    query,
		"simulate": fmt.Sprintf("%t", simulate),
	}

	body, status, err := c.postMultipartWithHeaders(ctx, "/api/v1/commands", nil, fields, "", "", nil, map[string]string{"Client-ID": clientID})
	if err != nil {
		return nil, err
	}
	if !isCommandStatusOK(status, simulate) {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// isCommandStatusOK reports whether status is an acceptable response for a
// command submission. 202 Accepted is always valid (async path). 200 OK is
// only valid for simulate requests (synchronous plan response).
func isCommandStatusOK(status int, simulate bool) bool {
	return status == http.StatusAccepted || (simulate && status == http.StatusOK)
}

// CancelCommands cancels running commands matching an optional query.
func (c *FormaeClient) CancelCommands(ctx context.Context, query string, clientID string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}

	body, status, err := c.do(ctx, request{
		Method:  http.MethodPost,
		Path:    "/api/v1/commands/cancel",
		Query:   q,
		Headers: map[string]string{"Client-ID": clientID},
	}, noRetry)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return c.route.collectionMiss(json.RawMessage(`{"CommandIds":[]}`))
	}
	if status != http.StatusAccepted {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListChangesSinceLastReconcile retrieves modifications since last reconcile for a stack.
func (c *FormaeClient) ListChangesSinceLastReconcile(ctx context.Context, stack string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/stacks/%s/changes-since-last-reconcile", url.PathEscape(stack))
	body, status, err := c.get(ctx, path, nil, retryOnce)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ForceSync triggers an immediate resource synchronization.
func (c *FormaeClient) ForceSync(ctx context.Context) error {
	_, status, err := c.post(ctx, "/api/v1/admin/synchronize", nil, noRetry)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("agent returned status %d", status)
	}

	return nil
}

// ForceDiscover triggers an immediate resource discovery.
func (c *FormaeClient) ForceDiscover(ctx context.Context) error {
	_, status, err := c.post(ctx, "/api/v1/admin/discover", nil, noRetry)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("agent returned status %d", status)
	}

	return nil
}

// ForceCheckTTL triggers an immediate TTL expiry sweep.
func (c *FormaeClient) ForceCheckTTL(ctx context.Context) (json.RawMessage, error) {
	body, status, err := c.post(ctx, "/api/v1/admin/check-ttl", nil, noRetry)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}
	return body, nil
}

// ForceReconcileStack triggers a one-shot reconcile for a specific stack.
// Returns the response body and HTTP status. On non-2xx status, error is non-nil
// but body is also returned so callers can surface the agent's error JSON.
func (c *FormaeClient) ForceReconcileStack(ctx context.Context, label string) (json.RawMessage, int, error) {
	path := fmt.Sprintf("/api/v1/stacks/%s/reconcile", url.PathEscape(label))
	body, status, err := c.post(ctx, path, nil, noRetry)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return body, status, fmt.Errorf("agent returned status %d", status)
	}
	return body, status, nil
}

func (c *FormaeClient) postMultipartWithHeaders(ctx context.Context, path string, query url.Values, fields map[string]string, fileField, fileName string, fileContent []byte, headers map[string]string) ([]byte, int, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, 0, fmt.Errorf("failed to write field %s: %w", k, err)
		}
	}

	if fileField != "" && fileContent != nil {
		fw, err := w.CreateFormFile(fileField, fileName)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create form file: %w", err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			return nil, 0, fmt.Errorf("failed to write file content: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, 0, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return c.do(ctx, request{
		Method:      http.MethodPost,
		Path:        path,
		Query:       query,
		Headers:     headers,
		Body:        &buf,
		ContentType: w.FormDataContentType(),
	}, noRetry)
}
