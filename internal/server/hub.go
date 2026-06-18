package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHubBaseURL = "https://hub.platform.engineering"

// HubClient reads the formae hub catalog API.
type HubClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHubClient() *HubClient {
	return &HubClient{
		baseURL:    defaultHubBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type HubPlugin struct {
	QualifiedName string `json:"qualifiedName"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Type          string `json:"type"`
	Category      string `json:"category"`
	Summary       string `json:"summary"`
	Originator    struct {
		Domain   string `json:"domain"`
		Verified bool   `json:"verified"`
	} `json:"originator"`
	LatestStable struct {
		Version string `json:"version"`
		Channel string `json:"channel"`
	} `json:"latestStable"`
}

type HubPluginDetail struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	License       string `json:"license"`
	Status        string `json:"status"`
	GithubRepoURL string `json:"github_repo_url"`
}

func (c *HubClient) SearchPlugins(query string) ([]HubPlugin, error) {
	u := c.baseURL + "/api/v1/plugins"
	if query != "" {
		u += "?q=" + url.QueryEscape(query)
	}
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("hub request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub returned status %d", resp.StatusCode)
	}
	var out struct {
		Results []HubPlugin `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode hub response: %w", err)
	}
	// Client-side filter (the public API ignores q on some deployments).
	if query == "" {
		return out.Results, nil
	}
	var filtered []HubPlugin
	for _, p := range out.Results {
		if containsFold(p.Name, query) || containsFold(p.Namespace, query) || containsFold(p.Category, query) {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func (c *HubClient) GetPlugin(name string) (HubPluginDetail, error) {
	var d HubPluginDetail
	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/plugins/" + url.PathEscape(name))
	if err != nil {
		return d, fmt.Errorf("hub request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return d, fmt.Errorf("hub returned status %d for plugin %q", resp.StatusCode, name)
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return d, fmt.Errorf("decode hub plugin: %w", err)
	}
	return d, nil
}

func containsFold(haystack, needle string) bool {
	return len(needle) == 0 ||
		len(haystack) >= len(needle) &&
			stringContainsFold(haystack, needle)
}

func stringContainsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
