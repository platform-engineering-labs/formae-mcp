package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHubBaseURL = "https://hub.platform.engineering"

// HubClient reads the formae hub catalog API.
type HubClient struct {
	baseURL       string
	githubBaseURL string
	httpClient    *http.Client
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

// --- Examples support ---

type Example struct {
	Name               string `json:"name"`
	LikelyTemplateStub bool   `json:"likelyTemplateStub"`
}

// ListExamplesResult wraps examples with the ref + version-match + trust info
// the caller must surface to the user (adversarial-review requirements).
type ListExamplesResult struct {
	Plugin             string    `json:"plugin"`
	RefUsed            string    `json:"refUsed"`            // tag or "" (default branch)
	VersionMatched     bool      `json:"versionMatched"`     // false → examples may not match pinned schema
	OriginatorDomain   string    `json:"originatorDomain"`   // from the hub catalog
	OriginatorVerified bool      `json:"originatorVerified"`
	Examples           []Example `json:"examples"`
}

// GetExampleResult wraps file contents with the same ref + trust info.
type GetExampleResult struct {
	Plugin             string            `json:"plugin"`
	Example            string            `json:"example"`
	RefUsed            string            `json:"refUsed"`
	VersionMatched     bool              `json:"versionMatched"`
	OriginatorDomain   string            `json:"originatorDomain"`
	OriginatorVerified bool              `json:"originatorVerified"`
	Files              map[string]string `json:"files"` // filename → content
}

const defaultGithubBaseURL = "https://api.github.com"

func (c *HubClient) githubBase() string {
	if c.githubBaseURL != "" {
		return c.githubBaseURL
	}
	return defaultGithubBaseURL
}

// ownerRepo parses "https://github.com/<owner>/<repo>" into owner, repo.
func ownerRepo(repoURL string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(repoURL, "https://github.com/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("cannot parse github repo url %q", repoURL)
	}
	return parts[0], parts[1], nil
}

// tagExists checks whether a git tag exists on the repo.
func (c *HubClient) tagExists(owner, repo, tag string) bool {
	u := fmt.Sprintf("%s/repos/%s/%s/git/refs/tags/%s", c.githubBase(), owner, repo, url.PathEscape(tag))
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// resolveRef returns the tag matching version (trying "v<version>" then
// "<version>"), or "" when none exists (caller falls back to default branch).
func (c *HubClient) resolveRef(owner, repo, version string) string {
	if version == "" {
		return ""
	}
	for _, cand := range []string{"v" + version, version} {
		if c.tagExists(owner, repo, cand) {
			return cand
		}
	}
	return ""
}

// listExamplesForRepo lists /examples entries at the given ref ("" = default branch).
func (c *HubClient) listExamplesForRepo(repoURL, ref string) ([]Example, error) {
	owner, repo, err := ownerRepo(repoURL)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/examples", c.githubBase(), owner, repo)
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned status %d listing examples", resp.StatusCode)
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode examples listing: %w", err)
	}
	var real, stub []Example
	for _, e := range entries {
		if e.Name == "PklProject" || e.Name == "README.md" {
			continue
		}
		ex := Example{Name: e.Name, LikelyTemplateStub: e.Name == "basic"}
		if ex.LikelyTemplateStub {
			stub = append(stub, ex)
		} else {
			real = append(real, ex)
		}
	}
	return append(real, stub...), nil
}

// listExamplesResolved resolves the version to a ref and lists examples there,
// falling back to the default branch (versionMatched=false) when no tag matches.
func (c *HubClient) listExamplesResolved(repoURL, version string) (ListExamplesResult, error) {
	var res ListExamplesResult
	owner, repo, err := ownerRepo(repoURL)
	if err != nil {
		return res, err
	}
	ref := c.resolveRef(owner, repo, version)
	res.RefUsed = ref
	res.VersionMatched = ref != ""
	exs, err := c.listExamplesForRepo(repoURL, ref)
	if err != nil {
		return res, err
	}
	res.Examples = exs
	return res, nil
}

// ListExamples resolves the plugin's repo + catalog trust info, then lists
// version-matched examples. version "" means "use the latest / default branch".
func (c *HubClient) ListExamples(pluginName, version string) (ListExamplesResult, error) {
	var res ListExamplesResult
	res.Plugin = pluginName
	d, err := c.GetPlugin(pluginName)
	if err != nil {
		return res, err
	}
	if d.GithubRepoURL == "" {
		return res, fmt.Errorf("plugin %q has no github_repo_url", pluginName)
	}
	// Trust info comes from the list endpoint (detail has no originator).
	if cat, err := c.SearchPlugins(pluginName); err == nil {
		for _, p := range cat {
			if p.Name == pluginName {
				res.OriginatorDomain = p.Originator.Domain
				res.OriginatorVerified = p.Originator.Verified
				if version == "" {
					version = p.LatestStable.Version
				}
				break
			}
		}
	}
	out, err := c.listExamplesResolved(d.GithubRepoURL, version)
	if err != nil {
		return res, err
	}
	out.Plugin = pluginName
	out.OriginatorDomain = res.OriginatorDomain
	out.OriginatorVerified = res.OriginatorVerified
	return out, nil
}

// GetExample fetches the PKL files in /examples/<name> at the version-matched ref.
func (c *HubClient) GetExample(pluginName, exampleName, version string) (GetExampleResult, error) {
	var res GetExampleResult
	res.Plugin = pluginName
	res.Example = exampleName

	d, err := c.GetPlugin(pluginName)
	if err != nil {
		return res, err
	}
	if d.GithubRepoURL == "" {
		return res, fmt.Errorf("plugin %q has no github_repo_url", pluginName)
	}

	// Trust info + version resolution from catalog.
	if cat, err := c.SearchPlugins(pluginName); err == nil {
		for _, p := range cat {
			if p.Name == pluginName {
				res.OriginatorDomain = p.Originator.Domain
				res.OriginatorVerified = p.Originator.Verified
				if version == "" {
					version = p.LatestStable.Version
				}
				break
			}
		}
	}

	owner, repo, err := ownerRepo(d.GithubRepoURL)
	if err != nil {
		return res, err
	}
	ref := c.resolveRef(owner, repo, version)
	res.RefUsed = ref
	res.VersionMatched = ref != ""

	// List the directory contents for /examples/<name>.
	u := fmt.Sprintf("%s/repos/%s/%s/contents/examples/%s", c.githubBase(), owner, repo, url.PathEscape(exampleName))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return res, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("github returned status %d listing example dir", resp.StatusCode)
	}
	var entries []struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return res, fmt.Errorf("decode example dir listing: %w", err)
	}

	files := make(map[string]string)
	for _, e := range entries {
		if e.Type != "file" || !strings.HasSuffix(e.Name, ".pkl") {
			continue
		}
		if e.DownloadURL == "" {
			continue
		}
		fileResp, err := c.httpClient.Get(e.DownloadURL)
		if err != nil {
			return res, fmt.Errorf("download %s: %w", e.Name, err)
		}
		if fileResp.StatusCode != http.StatusOK {
			fileResp.Body.Close()
			return res, fmt.Errorf("download %s returned status %d", e.Name, fileResp.StatusCode)
		}
		content, readErr := io.ReadAll(fileResp.Body)
		fileResp.Body.Close()
		if readErr != nil {
			return res, fmt.Errorf("read %s: %w", e.Name, readErr)
		}
		files[e.Name] = string(content)
	}
	res.Files = files
	return res, nil
}
