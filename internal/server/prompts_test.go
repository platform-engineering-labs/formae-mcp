package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPrompts(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	// List all prompts
	prompts, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts failed: %v", err)
	}
	if len(prompts.Prompts) != 7 {
		t.Errorf("expected 7 prompts, got %d", len(prompts.Prompts))
	}

	expectedPrompts := map[string]bool{
		"check_drift":           false,
		"discover_resources":    false,
		"import_resources":      false,
		"deploy_infrastructure": false,
		"patch_infrastructure":  false,
		"build_plugin":          false,
		"add_resource_type":     false,
	}
	for _, p := range prompts.Prompts {
		if _, ok := expectedPrompts[p.Name]; !ok {
			t.Errorf("unexpected prompt: %s", p.Name)
		}
		expectedPrompts[p.Name] = true
	}
	for name, found := range expectedPrompts {
		if !found {
			t.Errorf("missing prompt: %s", name)
		}
	}
}

func TestGetPrompt_CheckDrift(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name: "check_drift",
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	tc, ok := result.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Messages[0].Content)
	}
	if !strings.Contains(tc.Text, "drift") {
		t.Error("expected prompt to mention drift")
	}
}

func TestGetPrompt_ImportResources_WithQuery(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "import_resources",
		Arguments: map[string]string{"query": "type:AWS::S3::Bucket"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	tc := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(tc.Text, "managed:false type:AWS::S3::Bucket") {
		t.Error("expected query to include both managed:false and the custom filter")
	}
}

func TestGetPrompt_BuildPlugin(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "build_plugin",
		Arguments: map[string]string{"provider": "cloudflare"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	tc := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(tc.Text, "cloudflare") {
		t.Error("expected prompt to include provider name")
	}
}

func TestGetPrompt_DeployInfrastructure(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "deploy_infrastructure",
		Arguments: map[string]string{"file_path": "/my/forma.pkl"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	tc := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(tc.Text, "/my/forma.pkl") {
		t.Error("expected prompt to include file path")
	}
}
