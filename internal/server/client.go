package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// BasicAuth carries the HTTP basic credentials a profile declares for its
// agent. A nil *BasicAuth means the agent is unauthenticated, which is the
// case for every tailnet-only agent.
type BasicAuth struct {
	Username string
	Password string
}

// FormaeClient is a lightweight HTTP client for the formae agent REST API.
type FormaeClient struct {
	endpoint   string
	creds      *BasicAuth
	httpClient *http.Client
}

func NewFormaeClient(endpoint string, creds *BasicAuth) *FormaeClient {
	return &FormaeClient{
		endpoint: endpoint,
		creds:    creds,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do applies the profile's credentials, if any, and performs the request. Every
// request the client makes goes through here, so an agent behind basic auth is
// reachable from all of them rather than only the ones someone remembered.
func (c *FormaeClient) do(req *http.Request) (*http.Response, error) {
	if c.creds != nil {
		req.SetBasicAuth(c.creds.Username, c.creds.Password)
	}
	return c.httpClient.Do(req)
}

// doRead performs the request and reads the whole body, which is what every
// caller of get/post wants.
func (c *FormaeClient) doRead(req *http.Request) ([]byte, int, error) {
	resp, err := c.do(req)
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

func (c *FormaeClient) get(path string, query url.Values) ([]byte, int, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	return c.doRead(req)
}

func (c *FormaeClient) post(path string, query url.Values) ([]byte, int, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doRead(req)
}

// ListResources queries the agent for resources matching the given query string.
func (c *FormaeClient) ListResources(query string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}

	body, status, err := c.get("/api/v1/resources", q)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return json.RawMessage("[]"), nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListStacks retrieves all stacks from the agent.
func (c *FormaeClient) ListStacks() (json.RawMessage, error) {
	body, status, err := c.get("/api/v1/stacks", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return json.RawMessage("[]"), nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListPolicies retrieves all standalone policies from the agent.
func (c *FormaeClient) ListPolicies() (json.RawMessage, error) {
	body, status, err := c.get("/api/v1/policies", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return json.RawMessage("[]"), nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ListTargets queries the agent for targets matching the given query string.
func (c *FormaeClient) ListTargets(query string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}

	body, status, err := c.get("/api/v1/targets", q)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return json.RawMessage("[]"), nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// GetCommandStatus retrieves the status of a specific command.
func (c *FormaeClient) GetCommandStatus(commandID string, clientID string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("id", commandID)

	req, err := http.NewRequest("GET", c.endpoint+"/api/v1/commands/status"+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Client-ID", clientID)

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("command %s not found", commandID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// ListCommands retrieves command statuses matching an optional query.
func (c *FormaeClient) ListCommands(query string, maxResults string, clientID string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}
	if maxResults != "" {
		q.Set("max_results", maxResults)
	}

	req, err := http.NewRequest("GET", c.endpoint+"/api/v1/commands/status"+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Client-ID", clientID)

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return json.RawMessage(`{"Commands":[]}`), nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetAgentStats retrieves agent statistics.
func (c *FormaeClient) GetAgentStats() (json.RawMessage, error) {
	body, status, err := c.get("/api/v1/stats", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// CheckHealth checks if the agent is healthy.
func (c *FormaeClient) CheckHealth() error {
	_, status, err := c.get("/api/v1/health", nil)
	if err != nil {
		return fmt.Errorf("agent is not reachable: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("agent returned unhealthy status: %d", status)
	}

	return nil
}

// SubmitCommand submits a forma command (apply/destroy) to the agent.
func (c *FormaeClient) SubmitCommand(command string, mode string, simulate bool, force bool, formaJSON []byte, clientID string) (json.RawMessage, error) {
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

	body, status, err := c.postMultipartWithHeaders("/api/v1/commands", nil, fields, fileField, fileName, fileContent, map[string]string{"Client-ID": clientID})
	if err != nil {
		return nil, err
	}
	if !isCommandStatusOK(status, simulate) {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// DestroyByQuery submits a destroy-by-query command to the agent.
func (c *FormaeClient) DestroyByQuery(query string, simulate bool, clientID string) (json.RawMessage, error) {
	fields := map[string]string{
		"command":  "destroy",
		"query":    query,
		"simulate": fmt.Sprintf("%t", simulate),
	}

	body, status, err := c.postMultipartWithHeaders("/api/v1/commands", nil, fields, "", "", nil, map[string]string{"Client-ID": clientID})
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
func (c *FormaeClient) CancelCommands(query string, clientID string) (json.RawMessage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}

	req, err := http.NewRequest("POST", c.endpoint+"/api/v1/commands/cancel"+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Client-ID", clientID)

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return json.RawMessage(`{"CommandIds":[]}`), nil
	}
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// ListChangesSinceLastReconcile retrieves modifications since last reconcile for a stack.
func (c *FormaeClient) ListChangesSinceLastReconcile(stack string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/stacks/%s/changes-since-last-reconcile", url.PathEscape(stack))
	body, status, err := c.get(path, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
}

// ForceSync triggers an immediate resource synchronization.
func (c *FormaeClient) ForceSync() error {
	_, status, err := c.post("/api/v1/admin/synchronize", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("agent returned status %d", status)
	}

	return nil
}

// ForceDiscover triggers an immediate resource discovery.
func (c *FormaeClient) ForceDiscover() error {
	_, status, err := c.post("/api/v1/admin/discover", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("agent returned status %d", status)
	}

	return nil
}

// ForceCheckTTL triggers an immediate TTL expiry sweep.
func (c *FormaeClient) ForceCheckTTL() (json.RawMessage, error) {
	body, status, err := c.post("/api/v1/admin/check-ttl", nil)
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
func (c *FormaeClient) ForceReconcileStack(label string) (json.RawMessage, int, error) {
	path := fmt.Sprintf("/api/v1/stacks/%s/reconcile", url.PathEscape(label))
	body, status, err := c.post(path, nil)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return body, status, fmt.Errorf("agent returned status %d", status)
	}
	return body, status, nil
}

func (c *FormaeClient) postMultipartWithHeaders(path string, query url.Values, fields map[string]string, fileField, fileName string, fileContent []byte, headers map[string]string) ([]byte, int, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

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

	req, err := http.NewRequest("POST", u, &buf)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.do(req)
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
