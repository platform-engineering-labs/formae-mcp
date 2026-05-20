package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// withFixtureWorkspace cd's into the fixture root for the duration of the test
// and restores cwd afterward. The create_inline_policy tool resolves the
// workspace from the MCP server's CWD.
func withFixtureWorkspace(t *testing.T, fixture string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	target := filepath.Join("..", "..", "testdata", "policy", fixture)
	if err := os.Chdir(target); err != nil {
		t.Fatalf("chdir %s: %v", target, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestCreateInlinePolicySetTTL(t *testing.T) {
	withFixtureWorkspace(t, "lifeline_fixture")

	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		if filepath.Base(path) == "main.pkl" {
			return []byte(`{"Stacks":[{"Label":"lifeline"}]}`), nil
		}
		return []byte(`{"Stacks":[]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	session := connectTestServer(t, "http://localhost:1")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":         "lifeline",
			"policy_type":   "ttl",
			"operation":     "set",
			"ttl_seconds":   1200,
			"on_dependents": "abort",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)

	var out struct {
		FilePath             string   `json:"file_path"`
		Operation            string   `json:"operation"`
		PKLSnippet           string   `json:"pkl_snippet"`
		InsertionAnchorStart int      `json:"insertion_anchor_start"`
		InsertionAnchorEnd   int      `json:"insertion_anchor_end"`
		ImportsToAdd         []string `json:"imports_to_add"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, text)
	}

	if out.Operation != "create" {
		t.Errorf("expected operation create, got %s", out.Operation)
	}
	if !strings.HasSuffix(out.FilePath, "main.pkl") {
		t.Errorf("expected file ending in main.pkl, got %s", out.FilePath)
	}
	if !strings.Contains(out.PKLSnippet, "ttl = 20.min") {
		t.Errorf("snippet missing ttl value:\n%s", out.PKLSnippet)
	}
	// fixture has stack closing `}` on line 8.
	if out.InsertionAnchorStart != 8 || out.InsertionAnchorEnd != 8 {
		t.Errorf("expected anchor 8-8, got %d-%d", out.InsertionAnchorStart, out.InsertionAnchorEnd)
	}
}

func TestCreateInlinePolicyStackNotFound(t *testing.T) {
	withFixtureWorkspace(t, "lifeline_fixture")

	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"production"}]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":         "lifeline",
			"policy_type":   "ttl",
			"operation":     "set",
			"ttl_seconds":   60,
			"on_dependents": "abort",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result, got success: %s", textContent(t, result))
	}
}

func TestCreateInlinePolicyExplicitFormaFile(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "policy", "lifeline_fixture", "main.pkl"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"lifeline"}]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":         "lifeline",
			"policy_type":   "ttl",
			"operation":     "set",
			"ttl_seconds":   60,
			"on_dependents": "abort",
			"forma_file":    abs,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}
