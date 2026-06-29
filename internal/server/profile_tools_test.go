package server

import (
	"context"
	"os"
	"os/exec"
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

func TestWriteProfile_OverwriteNonActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	withFakeVersion(t, "0.87.0")
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", "staging.pkl"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("prod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", "prod.pkl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	session := connectTestServer(t, "http://forced:1")
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "write_profile",
		Arguments: map[string]any{"name": "staging", "content": "new-content"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(t, res))
	}
	got, _ := os.ReadFile(filepath.Join(dir, "profiles", "staging.pkl"))
	if string(got) != "new-content" {
		t.Errorf("expected overwrite, got %q", string(got))
	}
}

func TestWriteProfile_RefusesActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	withFakeVersion(t, "0.87.0")
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", "prod.pkl"), []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("prod\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	session := connectTestServer(t, "http://forced:1")
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "write_profile",
		Arguments: map[string]any{"name": "prod", "content": "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(textContent(t, res), "active profile") {
		t.Fatalf("expected refusal of active profile, got %v / %s", res.IsError, textContent(t, res))
	}
	got, _ := os.ReadFile(filepath.Join(dir, "profiles", "prod.pkl"))
	if string(got) != "orig" {
		t.Errorf("active profile must be unchanged, got %q", string(got))
	}
}

func TestWriteProfile_MissingProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	withFakeVersion(t, "0.87.0")
	session := connectTestServer(t, "http://forced:1")
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "write_profile",
		Arguments: map[string]any{"name": "ghost", "content": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for missing profile (overwrite-only)")
	}
}

func TestCreateAndUseProfile(t *testing.T) {
	// Requires the real formae 0.87.0 CLI on PATH (shell-out verbs).
	if _, err := exec.LookPath("formae"); err != nil {
		t.Skip("formae not on PATH")
	}
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	withFakeVersion(t, "0.87.0")

	session := connectTestServer(t, "http://forced:1")
	// create
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_profile", Arguments: map[string]any{"name": "staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("create_profile error: %s", textContent(t, res))
	}
	// use
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "use_profile", Arguments: map[string]any{"name": "staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("use_profile error: %s", textContent(t, res))
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "active")); !strings.Contains(string(got), "staging") {
		t.Errorf("active pointer not updated: %q", string(got))
	}
}
