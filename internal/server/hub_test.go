package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
