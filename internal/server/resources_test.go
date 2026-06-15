package server

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResources(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(resources.Resources) != 7 {
		t.Errorf("expected 7 resources, got %d", len(resources.Resources))
	}

	expectedURIs := map[string]bool{
		"formae://docs/query-syntax":    false,
		"formae://docs/concepts":        false,
		"formae://docs/pkl-primer":      false,
		"formae://docs/forma-anatomy":   false,
		"formae://docs/annotations":     false,
		"formae://docs/troubleshooting": false,
		"formae://docs/index":           false,
	}
	for _, r := range resources.Resources {
		if _, ok := expectedURIs[r.URI]; !ok {
			t.Errorf("unexpected resource URI: %s", r.URI)
		}
		expectedURIs[r.URI] = true
	}
	for uri, found := range expectedURIs {
		if !found {
			t.Errorf("missing resource: %s", uri)
		}
	}
}

// TestResourceDocLinksAreWellFormed guards against the stale plugin-SDK doc
// URLs that the MCP previously handed to AI assistants (causing 404s). On the
// docs site every SDK page lives under plugin-sdk/tutorial/ or
// plugin-sdk/reference/ — there is no top-level /reference/ path and tutorial
// pages are never directly under /plugin-sdk/<NN>-...
func TestResourceDocLinksAreWellFormed(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	docURIs := []string{
		"formae://docs/query-syntax",
		"formae://docs/concepts",
		"formae://docs/pkl-primer",
		"formae://docs/forma-anatomy",
		"formae://docs/annotations",
		"formae://docs/troubleshooting",
	}

	// Matches the two known-broken shapes:
	//  - top-level reference: .../en/latest/reference/...   (must be plugin-sdk/reference/)
	//  - tutorial page missing tutorial/: .../en/latest/plugin-sdk/02-schema/ (must be plugin-sdk/tutorial/...)
	brokenLink := regexp.MustCompile(`https?://docs\.formae\.io/en/latest/(reference/|plugin-sdk/\d)`)

	for _, uri := range docURIs {
		result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("ReadResource(%s) failed: %v", uri, err)
		}
		for _, c := range result.Contents {
			if m := brokenLink.FindAllString(c.Text, -1); m != nil {
				t.Errorf("%s emits stale/broken doc link(s): %v", uri, m)
			}
		}
	}
}

func TestReadResource_QuerySyntax(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/query-syntax",
	})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "Bluge") {
		t.Error("expected query syntax doc to mention Bluge")
	}
}

func TestReadResource_Concepts(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/concepts",
	})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "Stack") {
		t.Error("expected concepts doc to mention Stack")
	}
}

func TestReadResource_PklPrimer(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/pkl-primer",
	})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "PKL") {
		t.Error("expected pkl-primer doc to mention PKL")
	}
	if !strings.Contains(result.Contents[0].Text, "amends") {
		t.Error("expected pkl-primer doc to mention amends")
	}
}

func TestReadResource_FormaAnatomy(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/forma-anatomy",
	})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "Minimal forma") {
		t.Error("expected forma-anatomy doc to mention Minimal forma")
	}
}

func TestReadResource_Annotations(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/annotations",
	})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "ResourceHint") {
		t.Error("expected annotations doc to mention ResourceHint")
	}
	if !strings.Contains(result.Contents[0].Text, "ConfigFieldHint") {
		t.Error("expected annotations doc to mention ConfigFieldHint")
	}
}

func TestReadResource_Troubleshooting(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/troubleshooting",
	})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "plugin not found") {
		t.Error("expected troubleshooting doc to mention plugin not found")
	}
}
