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

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
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
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

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
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			// Return no version so skew notice is skipped.
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{}`)
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
				_, _ = fmt.Fprint(w, `[{"id":"r1","type":"AWS::S3::Bucket","label":"unmanaged-bucket","managed":false}]`)
			} else {
				_, _ = fmt.Fprint(w, `[{"id":"r1","type":"AWS::S3::Bucket","label":"my-bucket","managed":true}]`)
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
			_, _ = fmt.Fprint(w, `[{"label":"default","description":"Default stack","resource_count":5}]`)
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

func TestListPolicies(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `[{"Label":"ephemeral-1h","Type":"ttl","Config":{"TTLSeconds":3600,"OnDependents":"abort"},"AttachedStacks":["dev-stack-1"]}]`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_policies",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "ephemeral-1h") {
		t.Errorf("expected response to contain policy label, got: %s", text)
	}
}

func TestListPoliciesEmpty(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_policies",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success on 404, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if text != "[]" {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestListTargets(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/targets": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `[{"label":"prod-us-east-1","namespace":"AWS"}]`)
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
			_, _ = fmt.Fprint(w, `{"version":"0.1.0","managed_resources":42,"unmanaged_resources":7}`)
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
				_, _ = fmt.Fprint(w, `{"id":"cmd-123","status":"completed"}`)
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
			_, _ = fmt.Fprint(w, `{"Commands":[{"id":"cmd-1","status":"completed"}]}`)
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
			_, _ = fmt.Fprint(w, `{"CommandIds":["cmd-1"]}`)
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

func TestForceCheckTTL(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/admin/check-ttl": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"ExpiredStacks":["dev-feature"],"DestroyedCommands":["cmd-42"]}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "force_check_ttl",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "dev-feature") {
		t.Errorf("expected response to contain expired stack, got: %s", text)
	}
}

func TestForceReconcileStack(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/stacks/lifeline/reconcile": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprint(w, `{"CommandID":"cmd-99","Stack":"lifeline"}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "force_reconcile_stack",
		Arguments: map[string]any{"stack": "lifeline"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "cmd-99") {
		t.Errorf("expected response to contain command id, got: %s", text)
	}
}

func TestForceReconcileStackNoPolicy(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"POST /api/v1/stacks/lifeline/reconcile": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":"stack 'lifeline' does not have an auto-reconcile policy attached"}`)
		},
	})
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "force_reconcile_stack",
		Arguments: map[string]any{"stack": "lifeline"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result, got success: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "auto-reconcile policy") {
		t.Errorf("expected agent error message, got: %s", text)
	}
}

func TestForceReconcileStackMissingArg(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "force_reconcile_stack",
	})
	if err == nil {
		t.Fatal("expected schema validation error for missing stack argument")
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
			_, _ = fmt.Fprint(w, `{"id":"cmd-apply-1","status":"pending"}`)
		},
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			// Return no version so skew notice is skipped.
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{}`)
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
			_, _ = fmt.Fprint(w, `{"id":"cmd-destroy-1","status":"pending"}`)
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
			_, _ = fmt.Fprint(w, `{"ModifiedResources":[{"Stack":"production","Type":"AWS::S3::Bucket","Label":"my-bucket","Operation":"update"}]}`)
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
			_, _ = fmt.Fprint(w, `[{"Label":"production"},{"Label":"staging"}]`)
		},
		"GET /api/v1/stacks/production/changes-since-last-reconcile": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"ModifiedResources":[{"Stack":"production","Type":"AWS::S3::Bucket","Label":"prod-bucket","Operation":"update"}]}`)
		},
		"GET /api/v1/stacks/staging/changes-since-last-reconcile": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"ModifiedResources":[]}`)
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
			_, _ = fmt.Fprint(w, `{"ModifiedResources":[]}`)
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
			_, _ = fmt.Fprint(w, `{"error":"internal server error"}`)
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

func withFakeVersion(t *testing.T, v string) {
	t.Helper()
	featuregate.SetDetectForTest(v)
	t.Cleanup(func() { featuregate.SetDetectForTest("0.0.0") })
}

// --- connection resolution tests ---

// stubResolver stands in for the execctx resolver. The concrete one shells out
// to the CLI and its injection points are unexported outside its package.
type stubResolver struct {
	ec         execctx.Context
	err        error
	managed    bool
	sawProfile string
}

func (r *stubResolver) Resolve(_ context.Context, profileName string) (execctx.Context, error) {
	r.sawProfile = profileName
	return r.ec, r.err
}

func (r *stubResolver) Bin() string { return r.ec.FormaeBin }

func (r *stubResolver) Managed() bool { return r.managed }

// An explicit profile argument must reach the resolver. Without this the other
// cases would pass even if every call resolved the active profile.
func TestClientFor_ExplicitProfileReachesTheResolver(t *testing.T) {
	r := &stubResolver{ec: execctx.Context{
		ProfileName: "p",
		Conn:        config.Classic{URL: "http://p-host", Port: 7000},
	}}
	s := New("http://forced:1")
	s.ctxResolver = r

	c, err := s.clientFor(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if r.sawProfile != "p" {
		t.Errorf("resolver saw profile %q, want %q", r.sawProfile, "p")
	}
	if c.endpoint != "http://p-host:7000" {
		t.Errorf("endpoint = %q, want the profile endpoint", c.endpoint)
	}
}

// A hosted profile is recognised and refused. This build cannot authenticate,
// and routing without a credential would replace a comprehensible
// "unsupported" with a remote 401.
func TestClientFor_RefusesHosted(t *testing.T) {
	r := &stubResolver{ec: execctx.Context{
		ProfileName: "acme-prod",
		Conn: config.Hosted{
			Endpoint:     config.HostedOrigin,
			Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa",
		},
	}}
	s := New("")
	s.ctxResolver = r

	_, err := s.clientFor(context.Background(), "acme-prod")
	if err == nil {
		t.Fatal("expected a hosted profile to be refused")
	}
	if !strings.Contains(err.Error(), "acme-prod") {
		t.Errorf("error does not name the profile: %v", err)
	}
}

func TestClientFor_ClassicBuildsURLPort(t *testing.T) {
	r := &stubResolver{ec: execctx.Context{
		Conn: config.Classic{URL: "http://localhost", Port: 49684},
	}}
	s := New("")
	s.ctxResolver = r

	c, err := s.clientFor(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c.endpoint != "http://localhost:49684" {
		t.Errorf("endpoint = %q", c.endpoint)
	}
}

// A forced endpoint carries its own port, so it resolves to a classic
// connection with no port of its own to append.
func TestClientFor_ForcedEndpointIsClassicWithoutAPort(t *testing.T) {
	s := New("http://forced:1")

	ec, err := s.resolveCtx(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	classic, ok := ec.Conn.(config.Classic)
	if !ok {
		t.Fatalf("Conn = %#v, want a classic connection", ec.Conn)
	}
	if classic.Port != 0 {
		t.Errorf("Port = %d, want 0", classic.Port)
	}
	c, err := s.clientFor(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c.endpoint != "http://forced:1" {
		t.Errorf("endpoint = %q, want the forced endpoint", c.endpoint)
	}
}

// A formae below the floor fails resolution before any skew notice can be
// produced, so without this the upgrade flow sees no signal at all and reports
// that there is nothing to upgrade — dead-ending exactly the users who need it.
func TestResolveCtx_ExplainsAnInstallBelowTheFloor(t *testing.T) {
	tooOld := fmt.Errorf("%w: requires formae >= 0.89.0 (connected: 0.88.1)", featuregate.ErrTooOld)

	t.Run("managed", func(t *testing.T) {
		s := New("")
		s.ctxResolver = &stubResolver{
			ec:      execctx.Context{FormaeBin: "/home/u/.formae-ai/opt/bin/formae"},
			err:     tooOld,
			managed: true,
		}
		_, err := s.resolveCtx(context.Background(), "")
		if err == nil {
			t.Fatal("expected the floor refusal to surface")
		}
		if !strings.Contains(err.Error(), "/formae:upgrade") {
			t.Errorf("managed install should be pointed at the sudo-free upgrade: %v", err)
		}
		if !strings.Contains(err.Error(), "0.89.0") {
			t.Errorf("error should still name the requirement: %v", err)
		}
	})

	t.Run("the user's own", func(t *testing.T) {
		s := New("")
		s.ctxResolver = &stubResolver{
			ec:  execctx.Context{FormaeBin: "/opt/pel/bin/formae"},
			err: tooOld,
		}
		_, err := s.resolveCtx(context.Background(), "")
		if err == nil {
			t.Fatal("expected the floor refusal to surface")
		}
		if !strings.Contains(err.Error(), "/opt/pel/bin/formae") {
			t.Errorf("error should name the binary that is too old: %v", err)
		}
		if !strings.Contains(err.Error(), "will not change it") {
			t.Errorf("error should say we do not touch the user's own install: %v", err)
		}
	})
}

// Every other resolution failure passes through untouched.
func TestResolveCtx_LeavesOtherFailuresAlone(t *testing.T) {
	s := New("")
	s.ctxResolver = &stubResolver{err: fmt.Errorf("profile %q not found", "nope")}
	_, err := s.resolveCtx(context.Background(), "")
	if err == nil || strings.Contains(err.Error(), "/formae:upgrade") {
		t.Fatalf("unrelated failure should not be dressed as an upgrade prompt: %v", err)
	}
}
