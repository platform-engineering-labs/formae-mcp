package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestApplyFormaRecordsMessage(t *testing.T) {
	for _, message := range []string{"Keep incident capacity", ""} {
		t.Run(fmt.Sprintf("%q", message), func(t *testing.T) {
			submitted := false
			agent := mockAgent(t, map[string]http.HandlerFunc{
				"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
					submitted = true
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Fatal(err)
					}
					values, present := r.MultipartForm.Value["message"]
					if !present || len(values) != 1 || values[0] != message {
						t.Errorf("message = %v (present %t), want %q", values, present, message)
					}
					w.WriteHeader(http.StatusAccepted)
					_, _ = fmt.Fprint(w, `{"CommandId":"test-command"}`)
				},
				"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
					_, _ = fmt.Fprint(w, `{"Capabilities":["command-metadata"]}`)
				},
			})
			defer agent.Close()
			path := t.TempDir() + "/forma.json"
			if err := writeTestFile(path, `{"Resources":[]}`); err != nil {
				t.Fatal(err)
			}
			session := connectTestServer(t, agent.URL)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "apply_forma", Arguments: map[string]any{"file_path": path, "mode": "reconcile", "message": message},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("apply failed: %s", textContent(t, result))
			}
			if !submitted {
				t.Fatal("command was not submitted")
			}
		})
	}
}

func TestApplyMessageRejectsAgentWithoutMetadataSupport(t *testing.T) {
	submitted := false
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats":     func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{}`) },
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) { submitted = true; w.WriteHeader(http.StatusAccepted) },
	})
	defer agent.Close()
	path := t.TempDir() + "/forma.json"
	if err := writeTestFile(path, `{"Resources":[]}`); err != nil {
		t.Fatal(err)
	}
	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": path, "mode": "reconcile", "message": "important reason"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || submitted {
		t.Fatal("unsupported agent accepted a message it cannot retain")
	}
}
