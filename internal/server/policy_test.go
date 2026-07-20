package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func TestCreateInlinePolicyRejectsOldSchema(t *testing.T) {
	dir := t.TempDir()
	pklProject := `amends "pkl:Project"

dependencies {
  ["formae"] {
    uri = "package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.80.1"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "PklProject"), []byte(pklProject), 0o644); err != nil {
		t.Fatalf("write PklProject: %v", err)
	}
	forma := `amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	formaFile := filepath.Join(dir, "main.pkl")
	if err := os.WriteFile(formaFile, []byte(forma), 0o644); err != nil {
		t.Fatalf("write main.pkl: %v", err)
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
			"stack":       "lifeline",
			"policy_type": "ttl",
			"operation":   "set",
			"ttl_seconds": 1200,
			"forma_file":  formaFile,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (schema pin 0.80.1 predates policies)")
	}
	text := textContent(t, result)
	if !strings.Contains(text, "0.82.0") {
		t.Errorf("got:\n%s\nwant:\nan error naming the minimum version 0.82.0", text)
	}
	if strings.Contains(text, "Cannot find type") {
		t.Errorf("got:\n%s\nwant:\nan actionable message, not a raw PKL trace", text)
	}
}

func TestCreateInlinePolicyRefusesWhenStandaloneAttached(t *testing.T) {
	withFixtureWorkspace(t, "lifeline_fixture")

	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"lifeline"}],"Policies":[]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"Label":"ephemeral-1h","Type":"ttl",`+
				`"Config":{"Type":"ttl","TTLSeconds":3600,"OnDependents":"abort"},`+
				`"AttachedStacks":["lifeline"]}]`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":       "lifeline",
			"policy_type": "ttl",
			"operation":   "set",
			"ttl_seconds": 1200,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (a standalone TTL policy is attached to lifeline)")
	}
	text := textContent(t, result)
	if !strings.Contains(text, "ephemeral-1h") {
		t.Errorf("got:\n%s\nwant:\nan error naming the attached standalone", text)
	}
}

func TestCreateInlinePolicyAllowsDifferentStandaloneType(t *testing.T) {
	withFixtureWorkspace(t, "lifeline_fixture")

	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"lifeline"}],"Policies":[]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	// An auto-reconcile standalone is attached; setting an inline TTL is fine.
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"Label":"nightly-drift","Type":"auto-reconcile",`+
				`"Config":{"Type":"auto-reconcile","IntervalSeconds":300},`+
				`"AttachedStacks":["lifeline"]}]`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":       "lifeline",
			"policy_type": "ttl",
			"operation":   "set",
			"ttl_seconds": 1200,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("got:\nerror: %s\nwant:\nsuccess (the standalone is a different policy type)", textContent(t, result))
	}
}

func TestCreateInlinePolicyRemoveSkipsStandaloneCheck(t *testing.T) {
	withFixtureWorkspace(t, "lifeline_fixture")

	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"lifeline"}],"Policies":[]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	// No agent handler registered: removing must not call list_policies at all.
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":       "lifeline",
			"policy_type": "ttl",
			"operation":   "remove",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("got:\nerror: %s\nwant:\nsuccess (remove must not consult the agent)", textContent(t, result))
	}
}

// TestCreateInlinePolicyRefusesSourceOnlyStandalone is the pass-3 regression:
// a stack carries a same-type standalone resolvable in source (attached but not
// yet applied, so the agent inventory is empty). Setting an inline policy of
// that type must be refused.
func TestCreateInlinePolicyRefusesSourceOnlyStandalone(t *testing.T) {
	withFixtureWorkspace(t, "source_conflict_fixture") // stack "app" attaches ttl-a in source
	prevEval := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"app"}],` +
			`"Policies":[{"Label":"ttl-a","Type":"ttl"},{"Label":"ttl-b","Type":"ttl"}]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prevEval })

	// Agent knows nothing yet.
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[]`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_inline_policy",
		Arguments: map[string]any{
			"stack":       "app",
			"policy_type": "ttl",
			"operation":   "set",
			"ttl_seconds": 1200,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (app has a source-only standalone TTL attached)")
	}
	if !strings.Contains(textContent(t, result), "ttl-a") {
		t.Errorf("got:\n%s\nwant:\nan error naming the source-only standalone ttl-a", textContent(t, result))
	}
}
