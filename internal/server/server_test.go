package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mockAgent creates a test HTTP server that simulates the formae agent.
// The handler map keys are "METHOD /path" strings.
func mockAgent(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if h, ok := handlers[key]; ok {
			h(w, r)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
}

// connectTestServer creates a formae MCP server pointing at the mock agent,
// connects it via InMemoryTransport, and returns a client session for calling tools.
func connectTestServer(t *testing.T, agentURL string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	s := New(agentURL)

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

func textContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected content in result, got none")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// --- Read-only tool tests ---

func TestCheckHealth(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/health": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "check_health",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if text != "Formae agent is healthy and reachable." {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestCheckHealthUnreachable(t *testing.T) {
	// Point to a non-existent server
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "check_health",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error, got success: %s", textContent(t, result))
	}
}

func TestListResources(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query().Get("query")
			if query == "managed:false" {
				fmt.Fprint(w, `[{"id":"r1","type":"AWS::S3::Bucket","label":"unmanaged-bucket","managed":false}]`)
			} else {
				fmt.Fprint(w, `[{"id":"r1","type":"AWS::S3::Bucket","label":"my-bucket","managed":true}]`)
			}
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)

	t.Run("all resources", func(t *testing.T) {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "list_resources",
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got error: %s", textContent(t, result))
		}
		text := textContent(t, result)
		if text == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("with query", func(t *testing.T) {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "list_resources",
			Arguments: map[string]any{"query": "managed:false"},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got error: %s", textContent(t, result))
		}
		text := textContent(t, result)
		if text == "" {
			t.Error("expected non-empty result")
		}
	})
}

func TestListStacks(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stacks": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"label":"default","description":"Default stack","resource_count":5}]`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_stacks",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

func TestListTargets(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/targets": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"label":"prod-us-east-1","namespace":"AWS"}]`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_targets",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

func TestGetAgentStats(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"version":"0.1.0","managed_resources":42,"unmanaged_resources":7}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_agent_stats",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

func TestGetCommandStatus(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/commands/status": func(w http.ResponseWriter, r *http.Request) {
			id := r.URL.Query().Get("id")
			if id == "cmd-123" {
				fmt.Fprint(w, `{"id":"cmd-123","status":"completed"}`)
			} else {
				http.NotFound(w, r)
			}
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)

	t.Run("existing command", func(t *testing.T) {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "get_command_status",
			Arguments: map[string]any{"command_id": "cmd-123"},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got error: %s", textContent(t, result))
		}
	})

	t.Run("missing command_id rejected by schema", func(t *testing.T) {
		_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "get_command_status",
		})
		if err == nil {
			t.Fatal("expected schema validation error for missing command_id")
		}
	})
}

func TestListCommands(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/commands/status": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"Commands":[{"id":"cmd-1","status":"completed"}]}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_commands",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

// --- Mutation tool tests ---

func TestCancelCommands(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/commands/cancel": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"CommandIds":["cmd-1"]}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "cancel_commands",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

func TestForceSync(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/admin/synchronize": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "force_sync",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if text != "Resource synchronization triggered successfully." {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestForceDiscover(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/admin/discover": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "force_discover",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if text != "Resource discovery triggered successfully." {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestApplyForma_MissingFilePath(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "apply_forma",
		Arguments: map[string]any{"mode": "reconcile"},
	})
	if err == nil {
		t.Fatal("expected schema validation error for missing file_path")
	}
}

func TestApplyForma_MissingMode(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "apply_forma",
		Arguments: map[string]any{"file_path": "/tmp/test.json"},
	})
	if err == nil {
		t.Fatal("expected schema validation error for missing mode")
	}
}

func TestApplyForma_InvalidMode(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "apply_forma",
		Arguments: map[string]any{"file_path": "/tmp/test.json", "mode": "invalid"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid mode")
	}
}

func TestApplyForma_JSONFile(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"id":"cmd-apply-1","status":"pending"}`)
		},
	})
	defer agent.Close()

	// Create a temp JSON file
	tmpFile := t.TempDir() + "/test.json"
	if err := writeTestFile(tmpFile, `{"stacks":[{"label":"test"}]}`); err != nil {
		t.Fatal(err)
	}

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "apply_forma",
		Arguments: map[string]any{
			"file_path": tmpFile,
			"mode":      "reconcile",
			"simulate":  true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

func TestDestroyForma_MissingBothInputs(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "destroy_forma",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when both file_path and query are missing")
	}
}

func TestDestroyForma_BothInputsProvided(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "destroy_forma",
		Arguments: map[string]any{
			"file_path": "/tmp/test.json",
			"query":     "stack:test",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when both file_path and query are provided")
	}
}

func TestDestroyForma_ByQuery(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"id":"cmd-destroy-1","status":"pending"}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "destroy_forma",
		Arguments: map[string]any{
			"query":    "stack:staging",
			"simulate": true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

// --- Changes since last reconcile tests ---

func TestListChangesSinceLastReconcile_SingleStack(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stacks/production/changes-since-last-reconcile": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ModifiedResources":[{"Stack":"production","Type":"AWS::S3::Bucket","Label":"my-bucket","Operation":"update"}]}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_changes_since_last_reconcile",
		Arguments: map[string]any{"stack": "production"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if text == "" {
		t.Error("expected non-empty result")
	}
	if !strings.Contains(text, "my-bucket") {
		t.Errorf("expected result to contain 'my-bucket', got: %s", text)
	}
}

func TestListChangesSinceLastReconcile_AllStacks(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stacks": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"Label":"production"},{"Label":"staging"}]`)
		},
		"GET /api/v1/stacks/production/changes-since-last-reconcile": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ModifiedResources":[{"Stack":"production","Type":"AWS::S3::Bucket","Label":"prod-bucket","Operation":"update"}]}`)
		},
		"GET /api/v1/stacks/staging/changes-since-last-reconcile": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ModifiedResources":[]}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_changes_since_last_reconcile",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "production") || !strings.Contains(text, "staging") {
		t.Errorf("expected result to contain both stacks, got: %s", text)
	}
}

func TestListChangesSinceLastReconcile_NoChanges(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stacks/production/changes-since-last-reconcile": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ModifiedResources":[]}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_changes_since_last_reconcile",
		Arguments: map[string]any{"stack": "production"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
}

// --- Extract resources tests ---

func TestExtractResources_Registered(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "extract_resources" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("extract_resources tool not registered")
	}
}

func TestExtractResources_MissingQuery(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "extract_resources",
	})
	if err == nil {
		t.Fatal("expected schema validation error for missing query")
	}
}

// --- Agent error handling tests ---

func TestListResources_AgentError(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal server error"}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_resources",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for 500 response")
	}
}

func TestListResources_NotFound(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_resources",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success with empty array for 404")
	}
	text := textContent(t, result)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

// --- Helper ---

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
