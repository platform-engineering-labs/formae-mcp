package server

import (
	"context"
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
	if len(resources.Resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources.Resources))
	}

	expectedURIs := map[string]bool{
		"formae://docs/query-syntax": false,
		"formae://docs/concepts":     false,
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
