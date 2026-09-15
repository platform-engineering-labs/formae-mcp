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
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
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

func TestDriftGuidanceUsesPlainLanguageForNoCodeUsers(t *testing.T) {
	s := New("")
	r := codebase.Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return r, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	err := &commandHTTPError{status: 409, body: []byte(`{"error":"ReconcileRejected","data":{"ObservationID":"o","ModifiedStacks":{"blog":{"ModifiedResources":[{"ResourceID":"r"}]}}}}`)}
	text := allTextContent(t, s.applyErrorResult(context.Background(), ec, err))
	for _, unwanted := range []string{"since the last reconcile", "drift preference", "auto_absorb"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("user-facing guidance exposes internal term %q: %s", unwanted, text)
		}
	}
	for _, wanted := range []string{"changed outside formae", "protects the user from overwriting", "keep this change or revert it"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("missing plain-language guidance %q: %s", wanted, text)
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
		{"legacy external-only", "auto_absorb_external", "absorb", true, true, false},
		{"explicit auto", "auto_absorb", "absorb", true, true, false},
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
			if got := strings.Contains(string(raw), "For future changes,"); got != tc.want {
				t.Fatalf("offer=%v want=%v: %s", got, tc.want, raw)
			}
			if tc.want {
				for _, required := range []string{"explicitly chose keep", "set_drift_preference", "deletions", "formae patches", "auto_absorb", "No answer"} {
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

func TestResolutionPreviewOffersAutomaticReconcileAfterFirstRevert(t *testing.T) {
	s := New("")
	registry := codebase.Registry{Path: filepath.Join(t.TempDir(), "registry.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return registry, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	input := tools.ApplyFormaInput{Mode: "reconcile", Simulate: true, Context: &tools.SourceContext{Mode: "none"}, Resolution: &tools.DriftResolution{Decisions: []tools.DriftDecision{{ResourceID: "r", Action: "revert"}}}}
	result := s.keepPreferenceNotice(context.Background(), ec, input, []byte(`{"Review":{"ReviewID":"review"}}`))
	text := result
	for _, wanted := range []string{"automatically keep this stack aligned", "this stack", "all stacks", "decide separately for each stack", "create_inline_policy", "auto_reconcile"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("missing first-revert policy guidance %q: %s", wanted, text)
		}
	}
}

func TestResolutionPreviewDoesNotOfferNoCodePolicyForMaintainedCodebase(t *testing.T) {
	s := New("")
	registry := codebase.Registry{Path: filepath.Join(t.TempDir(), "registry.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return registry, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	input := tools.ApplyFormaInput{Mode: "reconcile", Simulate: true, Context: &tools.SourceContext{Mode: "codebase", BindingID: "binding"}, Resolution: &tools.DriftResolution{Decisions: []tools.DriftDecision{{ResourceID: "r", Action: "revert"}}}}
	if got := s.keepPreferenceNotice(context.Background(), ec, input, []byte(`{"Review":{"ReviewID":"review"}}`)); strings.Contains(got, "automatically keep this stack aligned") {
		t.Fatal("maintained codebase received no-code policy guidance")
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
	if strings.Contains(string(raw), "For future changes,") {
		t.Fatal("repeated preference offer for explicit prompt")
	}
}

func TestAutomaticAcceptanceIncludesPatchAndMixedObservations(t *testing.T) {
	s := New("")
	registry := codebase.Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return registry, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	id, _ := codebase.IdentityForConnection(ec.Conn)
	if _, err := registry.SetDriftPreference(context.Background(), id, "auto_absorb"); err != nil {
		t.Fatal(err)
	}
	remote := &commandHTTPError{status: 409, body: []byte(`{"error":"ReconcileRejected","data":{"ObservationID":"o","ModifiedStacks":{"s":{"ModifiedResources":[{"ResourceID":"r","ExternalChangesOnly":false,"ObservedCommand":"apply","ObservedMode":"patch"}]}}}}`)}
	raw, _ := json.Marshal(s.applyErrorResult(context.Background(), ec, remote))
	for _, part := range []string{"including formae patches", "decision-edit-conflict", "final combined preview"} {
		if !strings.Contains(string(raw), part) {
			t.Errorf("missing %q: %s", part, raw)
		}
	}
	if strings.Contains(string(raw), "Patches always require a decision") {
		t.Fatal("patches excluded from broad consent")
	}
}
