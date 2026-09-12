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
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

func TestDriftGuidanceRefreshesPreferenceAndRequiresPinnedResolution(t *testing.T) {
	s := New("")
	r := codebase.Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return r, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	id, _ := codebase.IdentityForConnection(ec.Conn)
	for _, observation := range []string{"", `"ObservationID":"observation",`} {
		body := []byte(`{"error":"ReconcileRejected","data":{` + observation + `"ModifiedStacks":{"s":{"ModifiedResources":[{"ResourceID":"r","ExternalChangesOnly":true}]}}}}`)
		err := &commandHTTPError{status: 409, body: body}
		initial := s.applyErrorResult(context.Background(), ec, err)
		encoded, _ := json.Marshal(initial)
		if !strings.Contains(string(encoded), "keep") {
			t.Fatal("missing decision language")
		}
		if _, e := r.SetDriftPreference(context.Background(), id, "auto_absorb_external"); e != nil {
			t.Fatal(e)
		}
		got := s.applyErrorResult(context.Background(), ec, err)
		value := got.StructuredContent.(map[string]any)
		workflow := value["workflow"].(map[string]any)
		if workflow["drift_preference"].(codebase.DriftPreference).Mode != "auto_absorb_external" {
			t.Fatal("stale preference")
		}
		if workflow["resolution_available"] != (observation != "") {
			t.Fatalf("wrong resolution availability: %#v", workflow)
		}
	}
}

func TestResolutionPreviewOffersPreferenceOnlyWhenUnset(t *testing.T) {
	for _, tc := range []struct {
		name, preference, action string
		simulate, review, want   bool
	}{
		{"unset keep", "", "absorb", true, true, true},
		{"explicit prompt", "prompt", "absorb", true, true, false},
		{"explicit auto", "auto_absorb_external", "absorb", true, true, false},
		{"revert", "", "revert", true, true, false},
		{"submission", "", "absorb", false, true, false},
		{"no review", "", "absorb", true, false, false},
		{"corrupt preference", "corrupt", "absorb", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := mockAgent(t, map[string]http.HandlerFunc{
				"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
					_, _ = fmt.Fprint(w, `{"Capabilities":["shared-drift-resolution"]}`)
				},
				"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
					if tc.review {
						_, _ = fmt.Fprint(w, `{"CommandId":"preview","Review":{"ReviewID":"review"}}`)
					} else {
						_, _ = fmt.Fprint(w, `{"CommandId":"preview"}`)
					}
				},
			})
			defer agent.Close()
			conn := config.Classic{URL: agent.URL}
			registry := codebase.Registry{Path: filepath.Join(t.TempDir(), "registry.json")}
			id, _ := codebase.IdentityForConnection(conn)
			if tc.preference == "corrupt" {
				if err := os.WriteFile(strings.TrimSuffix(registry.Path, ".json")+".preferences.json", []byte(`broken`), 0600); err != nil {
					t.Fatal(err)
				}
			} else if tc.preference != "" {
				if _, err := registry.SetDriftPreference(context.Background(), id, tc.preference); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "main.json")
			if err := os.WriteFile(path, []byte(`{"Resources":[]}`), 0600); err != nil {
				t.Fatal(err)
			}
			session := authoringSession(t, conn, agent.URL, authoringCLI(t), registry.Path)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": path, "mode": "reconcile", "simulate": tc.simulate, "resolution": map[string]any{"ObservationID": "o", "ReviewID": "review", "IdempotencyKey": "retry", "Decisions": []any{map[string]any{"ResourceID": "r", "Action": tc.action}}}}})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result)
			}
			raw, _ := json.Marshal(result)
			if got := strings.Contains(string(raw), "For future changes made outside formae"); got != tc.want {
				t.Fatalf("offer=%v want=%v: %s", got, tc.want, raw)
			}
			if tc.want {
				for _, required := range []string{"ExternalChangesOnly=true", "explicitly chose keep", "set_drift_preference", "external deletions", "Patches", "No answer"} {
					if !strings.Contains(string(raw), required) {
						t.Errorf("missing %q", required)
					}
				}
				pref, err := registry.DriftPreference(context.Background(), id)
				if err != nil || pref.Explicit {
					t.Fatalf("preview saved consent: %+v %v", pref, err)
				}
			}
		})
	}
}

func TestInitialDriftPreferenceOfferWaitsForKeep(t *testing.T) {
	s := New("")
	registry := codebase.Registry{Path: filepath.Join(t.TempDir(), "registry.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return registry, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	remote := &commandHTTPError{status: 409, body: []byte(`{"error":"ReconcileRejected","data":{"ObservationID":"o","ModifiedStacks":{}}}`)}
	result := s.applyErrorResult(context.Background(), ec, remote)
	raw, _ := json.Marshal(result)
	for _, part := range []string{"First ask only the current keep/revert question", "Only after the user explicitly chose keep"} {
		if !strings.Contains(string(raw), part) {
			t.Fatalf("missing ordering instruction %q: %s", part, raw)
		}
	}
	id, _ := codebase.IdentityForConnection(ec.Conn)
	if _, err := registry.SetDriftPreference(context.Background(), id, "prompt"); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(s.applyErrorResult(context.Background(), ec, remote))
	if strings.Contains(string(raw), "For future changes made outside formae") {
		t.Fatal("repeated preference offer for explicit prompt")
	}
}
