package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReadProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	withFakeVersion(t, "0.87.0")
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "cli { api { url = \"http://x\" port = 1 } }\n"
	if err := os.WriteFile(filepath.Join(dir, "profiles", "prod.pkl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	session := connectTestServer(t, "http://forced:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_profile",
		Arguments: map[string]any{"name": "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, result))
	}
	if !strings.Contains(textContent(t, result), "http://x") {
		t.Errorf("expected profile content, got %s", textContent(t, result))
	}
}

func TestReadProfile_VersionGate(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir())
	withFakeVersion(t, "0.86.0")
	session := connectTestServer(t, "http://forced:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_profile",
		Arguments: map[string]any{"name": "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(textContent(t, result), "requires formae >= 0.87.0") {
		t.Fatalf("expected version-gate error, got %v / %s", result.IsError, textContent(t, result))
	}
}
