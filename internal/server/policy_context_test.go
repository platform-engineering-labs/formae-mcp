// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

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

func policyScanCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "formae")
	script := `#!/usr/bin/env python3
import pathlib,sys
if sys.argv[1]=='--version':
 print('formae version 0.88.0')
 sys.exit(0)
assert sys.argv[1]=='eval'
source=pathlib.Path(sys.argv[2]+'.json')
if not source.exists():
 print('not a standalone forma',file=sys.stderr)
 sys.exit(1)
print(source.read_text())
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePolicyScanFile(t *testing.T, root, name, source, output string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if output != "" {
		if err := os.WriteFile(path+".json", []byte(output), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestScopedPolicyPlannerRejectsEveryScannedStackScope(t *testing.T) {
	withFakeVersion(t, "0.88.0")
	for _, tc := range []struct{ name, scope string }{
		{"explicit", `"Stacks":[{"Label":"other"}]`},
		{"resource", `"Resources":[{"Stack":"other"}]`},
		{"generator", `"Generators":[{"Stack":"other"}]`},
		{"implicit-default", `"Resources":[{"Label":"implicit"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := mockAgent(t, map[string]http.HandlerFunc{"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `[]`) }})
			defer agent.Close()
			root := contextProject(t)
			main := writePolicyScanFile(t, root, "main.pkl", "forma {\n}\n", `{"Stacks":[{"Label":"owned"}]}`)
			writePolicyScanFile(t, root, "other.pkl", "forma {\n}\n", "{"+tc.scope+`,"Policies":[{"Label":"shared","Type":"ttl"}]}`)
			session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, policyScanCLI(t), filepath.Join(t.TempDir(), "registry.json"))
			var binding codebase.Binding
			codebaseCall(t, session, "register_codebase", map[string]any{"path": root, "stacks": []string{"owned"}}, &binding)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_standalone_policy", Arguments: map[string]any{
				"label": "shared", "policy_type": "ttl", "ttl_seconds": 1200, "forma_file": main, "profile": "chosen",
				"context": map[string]any{"mode": "codebase", "binding_id": binding.ID},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(textContent(t, result), "scope") {
				t.Fatalf("planner used or silently skipped out-of-scope evaluated file: %s", textContent(t, result))
			}
			after, err := os.ReadFile(main)
			if err != nil || string(after) != "forma {\n}\n" {
				t.Fatal("planner changed source")
			}
		})
	}
}

func TestScopedPolicyPlannerStillSkipsPartialModules(t *testing.T) {
	withFakeVersion(t, "0.88.0")
	agent := mockAgent(t, map[string]http.HandlerFunc{"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `[]`) }})
	defer agent.Close()
	root := contextProject(t)
	main := writePolicyScanFile(t, root, "main.pkl", "forma {\n}\n", `{"Stacks":[{"Label":"owned"}]}`)
	writePolicyScanFile(t, root, "partial.pkl", "local value = 1\n", "")
	second := writePolicyScanFile(t, root, "policy.pkl", "forma {\n}\n", `{"Stacks":[{"Label":"owned"}],"Policies":[{"Label":"shared","Type":"ttl"}]}`)
	session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, policyScanCLI(t), filepath.Join(t.TempDir(), "registry.json"))
	var binding codebase.Binding
	codebaseCall(t, session, "register_codebase", map[string]any{"path": root, "stacks": []string{"owned"}}, &binding)
	var result struct {
		Operation string `json:"operation"`
		FilePath  string `json:"file_path"`
	}
	codebaseCall(t, session, "create_standalone_policy", map[string]any{"label": "shared", "policy_type": "ttl", "ttl_seconds": 1200, "forma_file": main, "context": map[string]any{"mode": "codebase", "binding_id": binding.ID}}, &result)
	if result.Operation != "noop" || result.FilePath != second {
		t.Fatalf("valid declaration not found after partial module: %+v", result)
	}
}

func TestPolicyConflictPlannersPropagateScanContextFailure(t *testing.T) {
	withFakeVersion(t, "0.88.0")
	for _, tool := range []string{"create_inline_policy", "attach_standalone_policy"} {
		t.Run(tool, func(t *testing.T) {
			agent := mockAgent(t, map[string]http.HandlerFunc{"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, `[{"Label":"new-policy","Type":"ttl","AttachedStacks":[]}]`)
			}})
			defer agent.Close()
			root := contextProject(t)
			source := `forma {
 new formae.Stack {
  label = "owned"
  policies = new Listing {
   new formae.PolicyResolvable { label = "shared" }
  }
 }
}
`
			main := writePolicyScanFile(t, root, "main.pkl", source, `{"Stacks":[{"Label":"owned"}]}`)
			writePolicyScanFile(t, root, "other.pkl", "forma {}\n", `{"Stacks":[{"Label":"other"}],"Policies":[{"Label":"shared","Type":"ttl"}]}`)
			session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, policyScanCLI(t), filepath.Join(t.TempDir(), "registry.json"))
			var binding codebase.Binding
			codebaseCall(t, session, "register_codebase", map[string]any{"path": root, "stacks": []string{"owned"}}, &binding)
			input := map[string]any{"stack": "owned", "forma_file": main, "context": map[string]any{"mode": "codebase", "binding_id": binding.ID}}
			if tool == "create_inline_policy" {
				input["policy_type"] = "ttl"
				input["operation"] = "set"
				input["ttl_seconds"] = 1200
			} else {
				input["policy_label"] = "new-policy"
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: input})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(textContent(t, result), "scope") {
				t.Fatalf("conflict planner swallowed context error: %s", textContent(t, result))
			}
		})
	}
}

func TestDisposablePolicyResolversPropagateContextFailures(t *testing.T) {
	for _, kind := range []string{"evaluated-scope", "file-path"} {
		t.Run(kind, func(t *testing.T) {
			agent := mockAgent(t, map[string]http.HandlerFunc{"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
			}})
			defer agent.Close()
			conn := config.Classic{URL: agent.URL}
			identity, err := codebase.IdentityForConnection(conn)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			main := writePolicyScanFile(t, root, "main.pkl", "forma {}\n", `{"Stacks":[{"Label":"owned"}]}`)
			other := writePolicyScanFile(t, root, "other.pkl", "forma {}\n", `{"Generators":[{"Stack":"other"}],"Policies":[{"Label":"shared"}]}`)
			if kind == "file-path" {
				outside := writePolicyScanFile(t, t.TempDir(), "other.pkl", "forma {}\n", `{"Stacks":[{"Label":"owned"}]}`)
				if err := os.Remove(other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, other); err != nil {
					t.Fatal(err)
				}
			}
			metadata, err := json.Marshal(authoringMetadata{Version: 1, Directory: root, Identity: identity, Stacks: []string{"owned"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, authoringMetadataName), metadata, 0600); err != nil {
				t.Fatal(err)
			}
			s := New("")
			s.gate = func() error { return nil }
			s.ctxResolver = authoringResolver{conn: conn, bin: policyScanCLI(t)}
			registry := codebase.Registry{Path: filepath.Join(t.TempDir(), "registry.json")}
			s.codebaseRegistry = func() (codebase.Registry, error) { return registry, nil }
			ctx, _, _, err := s.policyWorkspace(context.Background(), "chosen", &tools.SourceContext{Mode: "none", TemporaryDirectory: root}, main)
			if err != nil {
				t.Fatal(err)
			}
			eval := s.policyEval(ctx)
			// The predicate may see the valid main file, but never the invalid other file.
			visited := 0
			_, err = resolveFormaFileBy(root, eval, func(raw []byte) bool { visited++; return true })
			if !isSourceContextError(err) || visited != 1 {
				t.Fatalf("scope/path failure reached predicate or was skipped: visited=%d err=%v", visited, err)
			}
			_, err = resolveMainFormaFile(root, eval)
			if !isSourceContextError(err) {
				t.Fatalf("main resolver swallowed failure: %v", err)
			}
			_, _, err = s.standaloneTypeOf(ctx, "shared", nil, root)
			if !isSourceContextError(err) {
				t.Fatalf("policy type lookup swallowed failure: %v", err)
			}
		})
	}
}

func TestLegacyPolicyEvaluationStillIgnoresStackScope(t *testing.T) {
	root := t.TempDir()
	path := writePolicyScanFile(t, root, "legacy.pkl", "forma {}\n", `{"Stacks":[{"Label":"other"}]}`)
	s := New("")
	call := &policyCall{ec: execctx.Context{FormaeBin: policyScanCLI(t)}, source: nil}
	ctx := context.WithValue(context.Background(), policyCallKey{}, call)
	got, err := resolveStackFile(root, "other", s.policyEval(ctx))
	if err != nil || got != path {
		t.Fatalf("omitted-context legacy evaluation changed: %s %v", got, err)
	}
}
