package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectServer connects an already-built *Server via InMemoryTransport and
// returns a client session for calling tools. Prefer connectTestServer when
// the server can be built internally; use this when you need to inject
// dependencies (e.g. a stub HubClient) before connecting.
func connectServer(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	t1, t2 := mcp.NewInMemoryTransports()

	serverSession, err := s.mcpServer.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server.Connect failed: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })

	return clientSession
}

func TestHubClientSearchPlugins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/k8s","name":"k8s","namespace":"K8S","type":"resource","category":"containers","summary":"Kubernetes resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.1.5","channel":"stable"}}]}`))
	}))
	defer srv.Close()

	c := &HubClient{baseURL: srv.URL, httpClient: srv.Client()}
	plugins, err := c.SearchPlugins("")
	if err != nil {
		t.Fatalf("SearchPlugins: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name != "k8s" || plugins[0].Namespace != "K8S" {
		t.Fatalf("unexpected plugins: %+v", plugins)
	}
	if plugins[0].LatestStable.Version != "0.1.5" {
		t.Fatalf("want version 0.1.5, got %q", plugins[0].LatestStable.Version)
	}
	if !plugins[0].Originator.Verified || plugins[0].Originator.Domain != "platform.engineering" {
		t.Fatalf("originator not parsed: %+v", plugins[0].Originator)
	}
}

func TestHubClientSearchPluginsWithFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		// Assert that the query parameter is present in the request URL
		q := r.URL.Query().Get("q")
		if q != "k8s" {
			t.Errorf("expected q=k8s in URL, got q=%q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		// Return multiple plugins, only one matching the query
		_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/k8s","name":"k8s","namespace":"K8S","type":"resource","category":"containers","summary":"Kubernetes resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.1.5","channel":"stable"}},{"qualifiedName":"platform.engineering/aws","name":"aws","namespace":"AWS","type":"resource","category":"compute","summary":"AWS resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.2.0","channel":"stable"}}]}`))
	}))
	defer srv.Close()

	c := &HubClient{baseURL: srv.URL, httpClient: srv.Client()}
	plugins, err := c.SearchPlugins("k8s")
	if err != nil {
		t.Fatalf("SearchPlugins: %v", err)
	}
	// Should only return the k8s plugin, filtered by name
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %+v", len(plugins), plugins)
	}
	if plugins[0].Name != "k8s" {
		t.Fatalf("expected plugin name k8s, got %q", plugins[0].Name)
	}
	if plugins[0].Namespace != "K8S" {
		t.Fatalf("expected namespace K8S, got %q", plugins[0].Namespace)
	}
}

func TestHubClientSearchPluginsFilterByNamespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if q != "AWS" {
			t.Errorf("expected q=AWS in URL, got q=%q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		// Return multiple plugins, filtering by namespace (case-insensitive)
		_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/aws","name":"aws","namespace":"AWS","type":"resource","category":"compute","summary":"AWS resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.2.0","channel":"stable"}},{"qualifiedName":"platform.engineering/k8s","name":"k8s","namespace":"K8S","type":"resource","category":"containers","summary":"Kubernetes resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.1.5","channel":"stable"}}]}`))
	}))
	defer srv.Close()

	c := &HubClient{baseURL: srv.URL, httpClient: srv.Client()}
	plugins, err := c.SearchPlugins("AWS")
	if err != nil {
		t.Fatalf("SearchPlugins: %v", err)
	}
	// Should only return the AWS plugin, filtered by namespace (case-insensitive)
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %+v", len(plugins), plugins)
	}
	if plugins[0].Namespace != "AWS" {
		t.Fatalf("expected namespace AWS, got %q", plugins[0].Namespace)
	}
}

func TestHubClientSearchPluginsFilterByCategory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if q != "compute" {
			t.Errorf("expected q=compute in URL, got q=%q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		// Return multiple plugins, filtering by category
		_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/aws","name":"aws","namespace":"AWS","type":"resource","category":"compute","summary":"AWS resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.2.0","channel":"stable"}},{"qualifiedName":"platform.engineering/k8s","name":"k8s","namespace":"K8S","type":"resource","category":"containers","summary":"Kubernetes resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.1.5","channel":"stable"}}]}`))
	}))
	defer srv.Close()

	c := &HubClient{baseURL: srv.URL, httpClient: srv.Client()}
	plugins, err := c.SearchPlugins("compute")
	if err != nil {
		t.Fatalf("SearchPlugins: %v", err)
	}
	// Should only return the AWS plugin, filtered by category
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %+v", len(plugins), plugins)
	}
	if plugins[0].Category != "compute" {
		t.Fatalf("expected category compute, got %q", plugins[0].Category)
	}
}

func TestHubClientSearchPluginsNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query().Get("q")
		if q != "nonexistent" {
			t.Errorf("expected q=nonexistent in URL, got q=%q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		// Return multiple plugins, but none match the query
		_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/aws","name":"aws","namespace":"AWS","type":"resource","category":"compute","summary":"AWS resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.2.0","channel":"stable"}},{"qualifiedName":"platform.engineering/k8s","name":"k8s","namespace":"K8S","type":"resource","category":"containers","summary":"Kubernetes resource plugin","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.1.5","channel":"stable"}}]}`))
	}))
	defer srv.Close()

	c := &HubClient{baseURL: srv.URL, httpClient: srv.Client()}
	plugins, err := c.SearchPlugins("nonexistent")
	if err != nil {
		t.Fatalf("SearchPlugins: %v", err)
	}
	// Should return empty slice since no plugins match
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins, got %d: %+v", len(plugins), plugins)
	}
}

func TestHubClientGetPlugin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/k8s" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"name":"k8s","namespace":"K8S","license":"FSL-1.1-ALv2","status":"ready","github_repo_url":"https://github.com/platform-engineering-labs/formae-plugin-kubernetes"}`))
	}))
	defer srv.Close()

	c := &HubClient{baseURL: srv.URL, httpClient: srv.Client()}
	d, err := c.GetPlugin("k8s")
	if err != nil {
		t.Fatalf("GetPlugin: %v", err)
	}
	if d.GithubRepoURL != "https://github.com/platform-engineering-labs/formae-plugin-kubernetes" {
		t.Fatalf("unexpected repo url: %q", d.GithubRepoURL)
	}
}

// --- MCP tool tests ---

func TestSearchHubPluginsTool(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/k8s","name":"k8s","namespace":"K8S"}]}`))
	}))
	defer hub.Close()

	s := New("http://localhost:1")
	s.hub = &HubClient{baseURL: hub.URL, httpClient: hub.Client()}
	session := connectServer(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_hub_plugins",
		Arguments: map[string]any{"query": "k8s"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "K8S") {
		t.Fatalf("expected K8S in output, got: %s", text)
	}
}

func TestGetHubPluginTool(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/k8s") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"name":"k8s","namespace":"K8S","license":"FSL-1.1-ALv2","status":"ready","github_repo_url":"https://github.com/platform-engineering-labs/formae-plugin-kubernetes"}`))
	}))
	defer hub.Close()

	s := New("http://localhost:1")
	s.hub = &HubClient{baseURL: hub.URL, httpClient: hub.Client()}
	session := connectServer(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_hub_plugin",
		Arguments: map[string]any{"name": "k8s"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "github_repo_url") {
		t.Fatalf("expected github_repo_url in output, got: %s", text)
	}
	if !strings.Contains(text, "formae-plugin-kubernetes") {
		t.Fatalf("expected repo URL in output, got: %s", text)
	}
}

func TestHubClientListExamplesSortsAndFlags(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/contents/examples") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"name":"basic","type":"dir"},{"name":"eks-automode","type":"dir"},{"name":"stack-test.pkl","type":"file"}]`))
	}))
	defer gh.Close()

	c := &HubClient{githubBaseURL: gh.URL, httpClient: gh.Client()}
	exs, err := c.listExamplesForRepo("https://github.com/platform-engineering-labs/formae-plugin-aws", "")
	if err != nil {
		t.Fatalf("listExamplesForRepo: %v", err)
	}
	if exs[0].Name != "eks-automode" {
		t.Fatalf("expected real example first, got %q", exs[0].Name)
	}
	var basic *Example
	for i := range exs {
		if exs[i].Name == "basic" {
			basic = &exs[i]
		}
	}
	if basic == nil || !basic.LikelyTemplateStub {
		t.Fatalf("expected basic flagged as template stub, got %+v", basic)
	}
}

// Version-matched fetch: when a tag matching the version exists, it is used and
// versionMatched is true; the contents request must carry ?ref=<tag>.
func TestHubClientListExamplesVersionMatched(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/git/refs/tags/v0.1.5"):
			w.WriteHeader(http.StatusOK) // tag exists
			_, _ = w.Write([]byte(`{"ref":"refs/tags/v0.1.5"}`))
		case strings.HasSuffix(r.URL.Path, "/contents/examples"):
			if r.URL.Query().Get("ref") != "v0.1.5" {
				t.Errorf("expected ?ref=v0.1.5, got %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"name":"eks-automode","type":"dir"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gh.Close()

	c := &HubClient{githubBaseURL: gh.URL, httpClient: gh.Client()}
	res, err := c.listExamplesResolved("https://github.com/platform-engineering-labs/formae-plugin-aws", "0.1.5")
	if err != nil {
		t.Fatalf("listExamplesResolved: %v", err)
	}
	if !res.VersionMatched || res.RefUsed != "v0.1.5" {
		t.Fatalf("expected versionMatched at v0.1.5, got %+v", res)
	}
}

// No matching tag: fall back to default branch and flag versionMatched=false.
func TestHubClientListExamplesVersionFallback(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/git/refs/tags/"):
			w.WriteHeader(http.StatusNotFound) // no tags match
		case strings.HasSuffix(r.URL.Path, "/contents/examples"):
			if r.URL.Query().Get("ref") != "" {
				t.Errorf("expected no ref on fallback, got %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"name":"eks-automode","type":"dir"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gh.Close()

	c := &HubClient{githubBaseURL: gh.URL, httpClient: gh.Client()}
	res, err := c.listExamplesResolved("https://github.com/platform-engineering-labs/formae-plugin-aws", "0.9.9")
	if err != nil {
		t.Fatalf("listExamplesResolved: %v", err)
	}
	if res.VersionMatched {
		t.Fatalf("expected versionMatched=false on fallback, got %+v", res)
	}
}

// MCP-level tool test for list_plugin_examples.
func TestListPluginExamplesTool(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/plugins/aws":
			_, _ = w.Write([]byte(`{"name":"aws","namespace":"AWS","license":"FSL-1.1-ALv2","status":"ready","github_repo_url":"https://github.com/platform-engineering-labs/formae-plugin-aws"}`))
		case r.URL.Path == "/api/v1/plugins":
			_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/aws","name":"aws","namespace":"AWS","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.2.0","channel":"stable"}}]}`))
		case strings.Contains(r.URL.Path, "/git/refs/tags/"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/contents/examples"):
			_, _ = w.Write([]byte(`[{"name":"eks-automode","type":"dir"},{"name":"basic","type":"dir"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gh.Close()

	s := New("http://localhost:1")
	s.hub = &HubClient{
		baseURL:       gh.URL,
		githubBaseURL: gh.URL,
		httpClient:    gh.Client(),
	}
	session := connectServer(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_plugin_examples",
		Arguments: map[string]any{"plugin": "aws"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "eks-automode") {
		t.Fatalf("expected eks-automode in output, got: %s", text)
	}
	if !strings.Contains(text, "originatorDomain") {
		t.Fatalf("expected originatorDomain in output, got: %s", text)
	}
	if !strings.Contains(text, `"versionMatched":false`) {
		t.Fatalf("expected versionMatched:false in output (fallback path), got: %s", text)
	}
}

// MCP-level tool test for get_plugin_example.
func TestGetPluginExampleTool(t *testing.T) {
	const fileContent = `amends "package://platform.engineering/aws@0.2.0#/S3Bucket.pkl"`

	var ghStubURL string // filled in once the server is created

	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Hub: plugin detail
		case r.URL.Path == "/api/v1/plugins/aws":
			_, _ = w.Write([]byte(`{"name":"aws","namespace":"AWS","license":"FSL-1.1-ALv2","status":"ready","github_repo_url":"https://github.com/platform-engineering-labs/formae-plugin-aws"}`))
		// Hub: catalog (trust + version)
		case r.URL.Path == "/api/v1/plugins":
			_, _ = w.Write([]byte(`{"results":[{"qualifiedName":"platform.engineering/aws","name":"aws","namespace":"AWS","originator":{"domain":"platform.engineering","verified":true},"latestStable":{"version":"0.2.0","channel":"stable"}}]}`))
		// GitHub: tag ref — 404 to exercise default-branch fallback
		case strings.Contains(r.URL.Path, "/git/refs/tags/"):
			w.WriteHeader(http.StatusNotFound)
		// GitHub: example dir listing for /examples/s3-bucket
		case strings.HasSuffix(r.URL.Path, "/contents/examples/s3-bucket"):
			downloadURL := ghStubURL + "/raw/s3-bucket/main.pkl"
			body := `[{"name":"main.pkl","type":"file","download_url":"` + downloadURL + `"}]`
			_, _ = w.Write([]byte(body))
		// GitHub: raw file content
		case strings.HasSuffix(r.URL.Path, "/raw/s3-bucket/main.pkl"):
			_, _ = w.Write([]byte(fileContent))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gh.Close()
	ghStubURL = gh.URL

	s := New("http://localhost:1")
	s.hub = &HubClient{
		baseURL:       gh.URL,
		githubBaseURL: gh.URL,
		httpClient:    gh.Client(),
	}
	session := connectServer(t, s)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_plugin_example",
		Arguments: map[string]any{"plugin": "aws", "example": "s3-bucket"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "main.pkl") {
		t.Fatalf("expected main.pkl in output, got: %s", text)
	}
	if !strings.Contains(text, "S3Bucket.pkl") {
		t.Fatalf("expected file content in output, got: %s", text)
	}
	if !strings.Contains(text, `"originatorVerified":true`) {
		t.Fatalf("expected originatorVerified:true in output, got: %s", text)
	}
	if !strings.Contains(text, `"versionMatched":false`) {
		t.Fatalf("expected versionMatched:false (fallback path) in output, got: %s", text)
	}
}
