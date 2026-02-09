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

// FormaeClient is a lightweight HTTP client for the formae agent REST API.
type FormaeClient struct {
	endpoint   string
	httpClient *http.Client
}

func NewFormaeClient(endpoint string) *FormaeClient {
	return &FormaeClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *FormaeClient) get(path string, query url.Values) ([]byte, int, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}

func (c *FormaeClient) post(path string, query url.Values) ([]byte, int, error) {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	resp, err := c.httpClient.Post(u, "application/json", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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
	if status != http.StatusAccepted {
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
	if status != http.StatusAccepted {
		return nil, fmt.Errorf("agent returned status %d: %s", status, string(body))
	}

	return body, nil
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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

// ListDrift retrieves drift (modifications since last reconcile) for a stack.
func (c *FormaeClient) ListDrift(stack string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/stacks/%s/drift", url.PathEscape(stack))
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	return body, resp.StatusCode, nil
}
