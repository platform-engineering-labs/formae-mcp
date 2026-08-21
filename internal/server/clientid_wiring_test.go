package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/clientid"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// Command handlers must send the resolved CLI client ID, not a hardcoded
// constant, so commands issued through the MCP are attributed to the same
// client as the user's own CLI.
func TestCancelCommandsSendsResolvedClientID(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Client-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"CommandIds":[]}`))
	}))
	defer srv.Close()

	home := t.TempDir()
	s := New(srv.URL)
	s.clientID = &clientid.Resolver{
		Home:      func() (string, error) { return home, nil },
		ReadFile:  os.ReadFile,
		EnsureRun: func(string) error { return nil },
	}
	writeIDFile(t, home, "2N3x8aQdLmVp0rGhTzYwBcKfJe1\n")

	res, _, err := s.handleCancelCommands(context.Background(), nil, tools.CancelCommandsInput{Query: "stack=default"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler returned error result: %+v", res)
	}
	if gotHeader != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("Client-ID header = %q, want the resolved CLI ID", gotHeader)
	}
}

// writeIDFile creates <dir>/.pel/formae/cli_client_id with the given content.
func writeIDFile(t *testing.T, dir, content string) {
	t.Helper()
	idDir := filepath.Join(dir, ".pel", "formae")
	if err := os.MkdirAll(idDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(idDir, "cli_client_id"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
