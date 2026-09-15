// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
)

func TestResolutionTransportRetainsReviewAndRetry(t *testing.T) {
	calls := 0
	original := `{"Stacks":[{"Label":"production"}],"Resources":[],"Targets":[{"Config":{"n":9007199254740993}}]}`
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["shared-drift-resolution"]}`)
		},
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			calls++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			var resolution map[string]any
			if err := json.Unmarshal([]byte(r.FormValue("resolution")), &resolution); err != nil {
				t.Errorf("missing resolution: %v", err)
			}
			if resolution["ObservationID"] != "observation" {
				t.Errorf("resolution lost: %v", resolution)
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = f.Close() }()
			body, _ := io.ReadAll(f)
			if string(body) != original {
				t.Errorf("input changed: %s", body)
			}
			if r.FormValue("force") == "true" {
				t.Error("forced resolution")
			}
			if r.FormValue("simulate") == "true" {
				_, _ = fmt.Fprint(w, `{"CommandId":"preview","Review":{"ReviewID":"review"},"Simulation":{"Command":{"ResourceUpdates":[]}}}`)
			} else {
				if resolution["ReviewID"] != "review" || resolution["IdempotencyKey"] != "retry-key" {
					t.Errorf("real controls: %v", resolution)
				}
				values, present := r.MultipartForm.Value["message"]
				if !present || len(values) != 1 || values[0] != "" {
					t.Error("explicit empty message lost")
				}
				_, _ = fmt.Fprint(w, `{"CommandId":"accepted","Review":{"ReviewID":"review"}}`)
			}
		},
	})
	defer agent.Close()
	path := t.TempDir() + "/main.json"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	session := connectTestServer(t, agent.URL)
	controls := map[string]any{"ObservationID": "observation", "Decisions": []any{map[string]any{"ResourceID": "resource", "Action": "absorb"}}}
	input := map[string]any{"file_path": path, "mode": "reconcile", "simulate": true, "resolution": controls}
	var preview map[string]any
	codebaseCall(t, session, "apply_forma", input, &preview)
	if preview["Review"] == nil {
		t.Fatalf("missing review: %v", preview)
	}
	controls["ReviewID"] = "review"
	controls["IdempotencyKey"] = "retry-key"
	input["simulate"] = false
	input["message"] = ""
	for range 2 {
		codebaseCall(t, session, "apply_forma", input, nil)
	}
	if calls != 3 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestResolutionRejectsUnsupportedAndSurfacesStale(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(fmt.Sprint(supported), func(t *testing.T) {
			posted := 0
			agent := mockAgent(t, map[string]http.HandlerFunc{
				"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
					if supported {
						_, _ = fmt.Fprint(w, `{"Capabilities":["shared-drift-resolution"]}`)
					} else {
						_, _ = fmt.Fprint(w, `{}`)
					}
				},
				"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
					posted++
					w.WriteHeader(409)
					_, _ = fmt.Fprint(w, `{"error":"DriftResolutionRejected","data":{"Code":"stale-review","Reason":"changed","ResourceID":"resource"}}`)
				},
			})
			defer agent.Close()
			path := t.TempDir() + "/main.json"
			if err := os.WriteFile(path, []byte(`{"Resources":[]}`), 0600); err != nil {
				t.Fatal(err)
			}
			session := connectTestServer(t, agent.URL)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": path, "mode": "reconcile", "simulate": true, "resolution": map[string]any{"ObservationID": "o", "Decisions": []any{map[string]any{"ResourceID": "r", "Action": "revert"}}}}})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatal("accepted unsupported or stale request")
			}
			want := "shared-drift-resolution"
			if supported {
				want = "stale-review"
			}
			if !strings.Contains(textContent(t, result), want) {
				t.Fatalf("lost diagnostic: %s", textContent(t, result))
			}
			if (!supported && posted != 0) || (supported && posted != 1) {
				t.Fatalf("unexpected retries/writes: %d", posted)
			}
		})
	}
}

func TestMaintainedApplyValidatesEveryEvaluatedStack(t *testing.T) {
	posted := 0
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{}`) },
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			posted++
			w.WriteHeader(202)
			_, _ = fmt.Fprint(w, `{"CommandId":"ok"}`)
		},
	})
	defer agent.Close()
	session := codebaseTestSession(t, config.Classic{URL: agent.URL}, t.TempDir()+"/registry.json")
	project := contextProject(t)
	var binding codebase.Binding
	codebaseCall(t, session, "register_codebase", map[string]any{"path": project, "stacks": []string{"owned"}}, &binding)
	for _, body := range []string{
		`{"Stacks":[{"Label":"owned"},{"Label":"other"}]}`,
		`{"Stacks":[{"Label":"owned"}],"Resources":[{"Stack":"other"}]}`,
		`{"Stacks":[{"Label":"owned"}],"Generators":[{"Stack":"other"}]}`,
		`{"Resources":[{"Label":"implicit-default"}]}`,
	} {
		path := project + "/main.json"
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": path, "mode": "reconcile", "simulate": true, "context": map[string]any{"mode": "codebase", "binding_id": binding.ID}}})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError || !strings.Contains(textContent(t, result), "scope") {
			t.Fatalf("unsafe scope accepted: %s", textContent(t, result))
		}
	}
	if posted != 0 {
		t.Fatalf("out-of-scope calls submitted: %d", posted)
	}
	path := project + "/main.json"
	if err := os.WriteFile(path, []byte(`{"Stacks":[{"Label":"owned"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	codebaseCall(t, session, "apply_forma", map[string]any{"file_path": path, "mode": "reconcile", "simulate": true, "context": map[string]any{"mode": "codebase", "binding_id": binding.ID}}, nil)
	if posted != 1 {
		t.Fatal("valid selected project not submitted")
	}
}

func TestCommandDesiredDeltaIsPartialRecordedGuidance(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["shared-drift-resolution"]}`)
		},
		"GET /api/v1/commands/accepted/desired-delta": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Client-ID") == "" {
				t.Error("missing client attribution")
			}
			_, _ = fmt.Fprint(w, `{"CommandId":"accepted","State":"Success","Partial":true,"Forma":{"Resources":[{"Properties":{"count":9007199254740993,"secret":{"$value":"opaque","$visibility":"secret"}}}]},"DeletedResources":[{"ResourceID":"gone","Label":"deleted"}]}`)
		},
	})
	defer agent.Close()
	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_command_desired_delta", Arguments: map[string]any{"command_id": "accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(textContent(t, result))
	}
	raw := textContent(t, result)
	for _, part := range []string{`"Partial":true`, `9007199254740993`, `"$visibility":"secret"`, `"DeletedResources"`} {
		if !strings.Contains(raw, part) {
			t.Errorf("lost recorded guidance %s: %s", part, raw)
		}
	}
}
