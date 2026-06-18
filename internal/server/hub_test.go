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
