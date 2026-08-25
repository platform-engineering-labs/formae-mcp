package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDo_SendsSuppliedHeaders verifies that headers named by a request reach
// the agent unchanged.
func TestDo_SendsSuppliedHeaders(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Client-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	if _, _, err := c.do(context.Background(), request{
		Method:  "GET",
		Path:    "/api/v1/health",
		Headers: map[string]string{"Client-ID": "abc123"},
	}); err != nil {
		t.Fatalf("do: unexpected error: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("Client-ID header: want %q, got %q", "abc123", got)
	}
}

// TestDo_HonoursCancellation verifies that a cancelled context aborts the
// request instead of waiting for a server that never answers.
func TestDo_HonoursCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newTestFormaeClient(srv)
	if _, _, err := c.do(ctx, request{Method: "GET", Path: "/api/v1/health"}); err == nil {
		t.Fatal("do with a cancelled context: expected an error, got nil")
	}
}

// TestClientIDReachesTheAgent verifies that every method carrying a Client-ID
// still sends it once routed through the executor, including the two multipart
// paths where a wrapper must populate the header itself.
func TestClientIDReachesTheAgent(t *testing.T) {
	const clientID = "client-x"

	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Client-ID")
		w.Header().Set("Content-Type", "application/json")
		// Answer per endpoint: a cancel submission requires 202, and a command
		// submission requires 202 when it is not a simulate.
		switch r.URL.Path {
		case "/api/v1/commands", "/api/v1/commands/cancel":
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	ctx := context.Background()

	calls := []struct {
		name string
		path string
		call func() error
	}{
		{"GetCommandStatus", "/api/v1/commands/status", func() error {
			_, err := c.GetCommandStatus(ctx, "cmd-1", clientID)
			return err
		}},
		{"ListCommands", "/api/v1/commands/status", func() error {
			_, err := c.ListCommands(ctx, "", "10", clientID)
			return err
		}},
		{"CancelCommands", "/api/v1/commands/cancel", func() error {
			_, err := c.CancelCommands(ctx, "", clientID)
			return err
		}},
		{"SubmitCommand", "/api/v1/commands", func() error {
			_, err := c.SubmitCommand(ctx, "apply", "reconcile", false, false, []byte(`{}`), clientID)
			return err
		}},
		{"DestroyByQuery", "/api/v1/commands", func() error {
			_, err := c.DestroyByQuery(ctx, "stack=default", false, clientID)
			return err
		}},
	}

	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			seen = map[string]string{}
			if err := tc.call(); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if seen[tc.path] != clientID {
				t.Fatalf("%s: Client-ID on %s: want %q, got %q", tc.name, tc.path, clientID, seen[tc.path])
			}
		})
	}
}

// TestNoDirectHTTPConstruction pins the structural criterion: agent requests are
// built in exactly one place. A per-method audit is how one method ends up
// without a header the executor is supposed to guarantee.
func TestNoDirectHTTPConstruction(t *testing.T) {
	banned := []string{"http.NewRequest", "httpClient.Do", "http.Get(", "http.Post("}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" {
			continue
		}
		if name == "requests.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, b := range banned {
			if strings.Contains(string(src), b) {
				t.Errorf("%s constructs an HTTP request directly (%s); build it in requests.go instead", name, b)
			}
		}
	}
}
