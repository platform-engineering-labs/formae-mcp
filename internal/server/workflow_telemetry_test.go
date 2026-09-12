package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

type workflowTransport func(*http.Request) (*http.Response, error)

func (f workflowTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWorkflowTelemetryOptOutAndCategoricalPayload(t *testing.T) {
	enabled := false
	calls := 0
	reporter := newWorkflowTelemetry()
	reporter.clientID = func() (string, error) { return "test-client", nil }
	reporter.enabled = func(context.Context, execctx.Context) bool { return enabled }
	reporter.client.Transport = workflowTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event["event"] != "mcp_workflow_context" {
			t.Fatalf("event: %s", raw)
		}
		if strings.Contains(string(raw), "private.example") || strings.Contains(string(raw), "token") {
			t.Fatalf("private data: %s", raw)
		}
		props := event["properties"].(map[string]any)
		if props["codebase_mode"] != "none" || props["drift_preference"] != "auto_absorb_external" || props["drift_preference_explicit"] != true {
			t.Fatalf("properties: %#v", props)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})
	id := codebase.Identity{Kind: "classic", Endpoint: "https://private.example"}
	pref := codebase.DriftPreference{Mode: "auto_absorb_external", Explicit: true}
	reporter.capture(context.Background(), execctx.Context{}, id, "none", pref, "mcp_workflow_context")
	if calls != 0 {
		t.Fatal("reported while opted out")
	}
	enabled = true
	reporter.capture(context.Background(), execctx.Context{}, id, "none", pref, "mcp_workflow_context")
	reporter.capture(context.Background(), execctx.Context{}, id, "none", pref, "mcp_workflow_context")
	if calls != 1 {
		t.Fatalf("duplicate contexts counted: %d", calls)
	}
}

func TestWorkflowTelemetryOfflineDoesNotRetryEachContext(t *testing.T) {
	reporter := newWorkflowTelemetry()
	reporter.clientID = func() (string, error) { return "test-client", nil }
	checks, calls := 0, 0
	reporter.enabled = func(context.Context, execctx.Context) bool { checks++; return true }
	reporter.client.Transport = workflowTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("offline") })
	for i := 0; i < 3; i++ {
		reporter.capture(context.Background(), execctx.Context{}, codebase.Identity{Kind: "classic", Endpoint: "http://localhost"}, "none", codebase.DriftPreference{Mode: "prompt"}, "mcp_workflow_context")
	}
	if checks != 1 || calls != 1 {
		t.Fatalf("repeated reporting work: checks=%d calls=%d", checks, calls)
	}
}

func TestWorkflowTelemetrySeparatesLocalhostOnDifferentMachines(t *testing.T) {
	w := newWorkflowTelemetry()
	w.enabled = func(context.Context, execctx.Context) bool { return true }
	client := "first-machine"
	w.clientID = func() (string, error) { return client, nil }
	ids := map[string]bool{}
	w.client.Transport = workflowTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), client) {
			t.Fatal("reported raw client identity")
		}
		var event struct {
			DistinctID string `json:"distinct_id"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		ids[event.DistinctID] = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})
	for _, value := range []string{"first-machine", "second-machine"} {
		client = value
		w.capture(context.Background(), execctx.Context{}, codebase.Identity{Kind: "classic", Endpoint: "http://localhost:49684"}, "none", codebase.DriftPreference{Mode: "prompt"}, "mcp_workflow_context")
	}
	if len(ids) != 2 || ids[""] {
		t.Fatalf("different machines collapsed: %#v", ids)
	}
}

func TestWorkflowTelemetryReadsExistingOptOut(t *testing.T) {
	for _, tc := range []struct {
		name, disabled, endpoint string
		want                     bool
	}{
		{"enabled", `false`, "http://localhost", true},
		{"disabled", `true`, "http://localhost", false},
		{"unknown", `null`, "http://localhost", false},
		{"changed-installation", `false`, "http://elsewhere", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "formae")
			body := `{"schemaVersion":1,"profile":"chosen","cli":{"disableUsageReporting":` + tc.disabled + `,"connection":{"mode":"classic","url":"` + tc.endpoint + `","port":1234}}}`
			script := "#!/bin/sh\n[ \"$1 $2 $3\" = \"profile show chosen\" ] || exit 1\nprintf '%s' '" + body + "'\n"
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ec := execctx.Context{FormaeBin: bin, ProfileName: "chosen", Conn: config.Classic{URL: "http://localhost", Port: 1234}}
			if got := workflowUsageEnabled(context.Background(), ec); got != tc.want {
				t.Fatalf("enabled=%v want %v", got, tc.want)
			}
		})
	}
}
