// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

type authoringResolver struct {
	conn config.Connection
	bin  string
}

func (r authoringResolver) Resolve(_ context.Context, profile string, _ bool) (execctx.Context, error) {
	return execctx.Context{ProfileName: profile, Conn: r.conn, FormaeBin: r.bin}, nil
}
func (r authoringResolver) Bin() string   { return r.bin }
func (r authoringResolver) Managed() bool { return false }
func authoringSession(t *testing.T, conn config.Connection, endpoint, bin, registry string) *mcp.ClientSession {
	t.Helper()
	s := New("")
	s.clientID = testClientIDResolver(t)
	s.gate = func() error { return nil }
	s.ctxResolver = authoringResolver{conn: conn, bin: bin}
	s.codebaseRegistry = func() (codebase.Registry, error) { return codebase.Registry{Path: registry}, nil }
	s.newClient = func(_ execctx.Context) (*FormaeClient, error) { return NewFormaeClient(endpoint), nil }
	a, b := mcp.NewInMemoryTransports()
	ss, err := s.mcpServer.Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: "authoring-test", Version: "0"}, nil)
	session, err := c.Connect(context.Background(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
func authoringCLI(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/formae"
	script := `#!/usr/bin/env python3
import sys,json,pathlib
args=sys.argv[1:]
if args[0]=='extract':
 assert args[1:3]==['--from-json','-'], args
 assert '--profile' not in args and '--config' not in args, args
 bundle=json.load(sys.stdin)
 assert bundle['Plugins'][0]['namespace']=='test', bundle
 assert bundle['Forma']['Extraction']['CompleteStacks'][0]['Label']=='owned'
 assert bundle['Forma']['Targets'][0]['Config']['number']==9007199254740993
 path=pathlib.Path(args[-1]);path.write_text(json.dumps(bundle['Forma']))
 (path.parent/'PklProject').write_text('amends "pkl:Project"\n')
elif args[0]=='eval':
 print(pathlib.Path(args[1]).read_text())
elif args[0]=='--version':
 print('formae version 0.0.0')
else:
 sys.exit(1)
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestDisposableAuthoringSurvivesFreshServerAndRejectsWrongInstallation(t *testing.T) {
	posts := 0
	desired := 0
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			desired++
			if r.URL.Query().Get("state") != "desired" || r.URL.Query().Get("query") != `stack:"owned"` {
				t.Errorf("not desired complete scope: %s", r.URL)
			}
			_, _ = fmt.Fprint(w, `{"Stacks":[{"Label":"owned"}],"Targets":[{"Label":"target","Config":{"number":9007199254740993}}],"Resources":[],"Extraction":{"CompleteStacks":[{"Label":"owned"}]}}`)
		},
		"GET /api/v1/plugins": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"plugins":[{"type":"resource","namespace":"test","name":"test","installedVersion":"1.2.3","localPath":"/remote/never-local"}]}`)
		},
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			posts++
			w.WriteHeader(202)
			_, _ = fmt.Fprint(w, `{"CommandId":"ok"}`)
		},
	})
	defer agent.Close()
	bin := authoringCLI(t)
	registry := t.TempDir() + "/registry.json"
	dir := t.TempDir()
	conn := config.Hosted{Endpoint: config.HostedOrigin, Installation: "111111111111111111111111111"}
	session := authoringSession(t, conn, agent.URL, bin, registry)
	var result struct {
		FilePath string         `json:"file_path"`
		Context  map[string]any `json:"context"`
	}
	codebaseCall(t, session, "prepare_authoring", map[string]any{"temporary_directory": dir, "stacks": []string{"owned"}}, &result)
	if result.FilePath != filepath.Join(dir, "main.pkl") || result.Context["mode"] != "none" {
		t.Fatalf("invalid artifact: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "PklProject")); err != nil {
		t.Fatal("dependencies missing", err)
	}
	if _, err := os.Stat(registry); !os.IsNotExist(err) {
		t.Fatal("disposable source registered")
	}
	if desired != 1 {
		t.Fatalf("extraction count %d", desired)
	}
	fresh := authoringSession(t, conn, agent.URL, bin, registry)
	codebaseCall(t, fresh, "apply_forma", map[string]any{"file_path": result.FilePath, "context": result.Context, "mode": "reconcile", "simulate": true}, nil)
	other := conn
	other.Installation = "222222222222222222222222222"
	wrong := authoringSession(t, other, agent.URL, bin, registry)
	response, err := wrong.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": result.FilePath, "context": result.Context, "mode": "reconcile", "simulate": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !response.IsError || !strings.Contains(textContent(t, response), "installation") {
		t.Fatalf("installation mismatch accepted: %s", textContent(t, response))
	}
	if posts != 1 {
		t.Fatalf("wrong submission count %d", posts)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	response, err = fresh.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": result.FilePath, "context": result.Context, "mode": "reconcile", "simulate": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !response.IsError || posts != 1 {
		t.Fatal("deleted workspace was accepted")
	}
}

func TestPrepareAuthoringRefusesOldAgentBeforeCreatingFiles(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{}`) }})
	defer agent.Close()
	dir := t.TempDir()
	session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, authoringCLI(t), t.TempDir()+"/registry.json")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "prepare_authoring", Arguments: map[string]any{"temporary_directory": dir, "stacks": []string{"owned"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(textContent(t, result), "desired-stack-extraction") {
		t.Fatalf("old agent not refused: %s", textContent(t, result))
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported preparation created files")
	}
}

func TestDisposableApplyChecksCurrentCapability(t *testing.T) {
	posts := 0
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{}`) },
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			posts++
			w.WriteHeader(202)
			_, _ = fmt.Fprint(w, `{"CommandId":"unsafe"}`)
		},
	})
	defer agent.Close()
	conn := config.Classic{URL: agent.URL}
	identity, err := codebase.IdentityForConnection(conn)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	raw, _ := json.Marshal(authoringMetadata{Version: 1, Directory: dir, Identity: identity, Stacks: []string{"owned"}})
	if err := os.WriteFile(dir+"/"+authoringMetadataName, raw, 0600); err != nil {
		t.Fatal(err)
	}
	file := dir + "/main.json"
	if err := os.WriteFile(file, []byte(`{"Stacks":[{"Label":"owned"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	session := authoringSession(t, conn, agent.URL, authoringCLI(t), t.TempDir()+"/registry.json")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": file, "context": map[string]any{"mode": "none", "temporary_directory": dir}, "mode": "reconcile", "simulate": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || posts != 0 {
		t.Fatal("old agent accepted no-codebase request")
	}
}

func TestAuthoringRealCLIEmptyStackTargetRoundTrip(t *testing.T) {
	bin := os.Getenv("FORMAE_INTEGRATION_BIN")
	coreProject := os.Getenv("FORMAE_INTEGRATION_CORE_PROJECT")
	if bin == "" || coreProject == "" {
		t.Skip("set FORMAE_INTEGRATION_BIN and FORMAE_INTEGRATION_CORE_PROJECT for real CLI/Pkl integration")
	}
	// Test-only schema bootstrap: an unpublished dev CLI cannot fetch its core
	// package from the hub during the renderer's immediate project resolve.
	wrapper := filepath.Join(t.TempDir(), "formae")
	script := "#!/usr/bin/env python3\nimport sys,pathlib,os\nargs=sys.argv[1:]\nif args[0]=='extract':\n p=pathlib.Path(args[-1]).parent/'PklProject'\n p.write_text(" + strconv.Quote("amends \"pkl:Project\"\ndependencies { [\"formae\"] = import("+strconv.Quote(coreProject)+") }\n") + ")\nos.execv(" + strconv.Quote(bin) + ",[" + strconv.Quote(bin) + "]+args)\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	bin = wrapper
	posts := 0
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/stacks": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `[]`) },
		"GET /api/v1/targets": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `[{"Label":"target","Namespace":"test","Config":{"number":9007199254740993}}]`)
		},
		"GET /api/v1/plugins": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{"plugins":[]}`) },
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Stacks":[{"Label":"stack","Policies":[{"Type":"ttl","TTLSeconds":3600,"OnDependents":"abort"}]}],"Targets":[{"Label":"target","Namespace":"test","Config":{"number":9007199254740993,"password":{"$gen":true,"$label":"external-password","$stack":"external","$output":"value"}}}],"Generators":[{"Type":"password","Label":"password","Stack":"stack","Length":20,"Uppercase":true,"Lowercase":true,"Digits":true,"Symbols":false,"ExcludeCharacters":"","RequireEachIncludedType":true}],"Extraction":{"CompleteStacks":[{"Label":"stack"}],"ReferenceGenerators":[{"Type":"password","Label":"external-password","Stack":"external","Length":20,"Uppercase":true,"Lowercase":true,"Digits":true,"Symbols":false,"ExcludeCharacters":"","RequireEachIncludedType":true}]}}`)
		},
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			posts++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = f.Close() }()
			raw, err := io.ReadAll(f)
			if err != nil {
				t.Error(err)
				return
			}
			var forma struct {
				Stacks  []struct{ Label string }
				Targets []struct{ Config json.RawMessage }
			}
			if err := json.Unmarshal(raw, &forma); err != nil {
				t.Error(err)
				return
			}
			if len(forma.Stacks) != 1 || forma.Stacks[0].Label != "stack" || len(forma.Targets) != 1 || !bytes.Contains(forma.Targets[0].Config, []byte("9007199254740993")) {
				t.Errorf("lost complete empty stack/target/numeric fidelity: %s", raw)
			}
			if posts == 2 {
				for _, part := range []string{`"Generators"`, `"Policies"`, `"$gen":true`, `"$stack":"external"`} {
					if !bytes.Contains(raw, []byte(part)) {
						t.Errorf("lost generator/policy/reference context %s: %s", part, raw)
					}
				}
				stacks, err := evaluatedStacks(raw)
				if err != nil || len(stacks) != 1 || stacks[0] != "stack" {
					t.Errorf("external generator became managed scope: %v %v", stacks, err)
				}
			}
			_, _ = fmt.Fprint(w, `{"CommandId":"preview","Simulation":{"Command":{"ResourceUpdates":[]}}}`)
		},
	})
	defer agent.Close()
	dir := t.TempDir()
	registry := t.TempDir() + "/registry.json"
	conn := config.Classic{URL: agent.URL}
	session := authoringSession(t, conn, agent.URL, bin, registry)
	var result authoringResult
	codebaseCall(t, session, "prepare_authoring", map[string]any{"temporary_directory": dir, "new_stacks": []string{"stack"}, "targets": []string{"target"}, "profile": "missing-offline-profile"}, &result)
	// A dev CLI has no published core schema version. The harness supplies its
	// explicit local schema dependency, exactly as local project authoring does.
	project := "amends \"pkl:Project\"\ndependencies { [\"formae\"] = import(" + strconv.Quote(coreProject) + ") }\n"
	if err := os.WriteFile(result.ProjectPath, []byte(project), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("pkl", "project", "resolve", dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("resolve: %v\n%s", err, output)
	}
	fresh := authoringSession(t, conn, agent.URL, bin, registry)
	codebaseCall(t, fresh, "apply_forma", map[string]any{"file_path": result.FilePath, "context": result.Context, "mode": "reconcile", "simulate": true, "profile": "also-missing-profile"}, nil)
	if posts != 1 {
		t.Fatalf("submitted %d previews", posts)
	}
	second := t.TempDir()
	codebaseCall(t, session, "prepare_authoring", map[string]any{"temporary_directory": second, "stacks": []string{"stack"}}, &result)
	codebaseCall(t, fresh, "apply_forma", map[string]any{"file_path": result.FilePath, "context": result.Context, "mode": "reconcile", "simulate": true}, nil)
	if posts != 2 {
		t.Fatalf("submitted %d previews", posts)
	}
}

func TestPolicyPlannerUsesSelectedDisposableSource(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `[]`) },
	})
	defer agent.Close()
	conn := config.Classic{URL: agent.URL}
	identity, _ := codebase.IdentityForConnection(conn)
	dir := t.TempDir()
	raw, _ := json.Marshal(authoringMetadata{Version: 1, Directory: dir, Identity: identity, Stacks: []string{"lifeline"}})
	if err := os.WriteFile(dir+"/"+authoringMetadataName, raw, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile("../../testdata/policy/lifeline_fixture/main.pkl")
	if err != nil {
		t.Fatal(err)
	}
	path := dir + "/main.pkl"
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir() + "/formae"
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' '{\"Stacks\":[{\"Label\":\"lifeline\"}]}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	session := authoringSession(t, conn, agent.URL, bin, t.TempDir()+"/registry.json")
	var plan struct {
		FilePath   string `json:"file_path"`
		PKLSnippet string `json:"pkl_snippet"`
	}
	codebaseCall(t, session, "create_inline_policy", map[string]any{"stack": "lifeline", "policy_type": "ttl", "operation": "set", "ttl_seconds": 1200, "forma_file": path, "profile": "chosen", "context": map[string]any{"mode": "none", "temporary_directory": dir}}, &plan)
	if plan.FilePath != path || !strings.Contains(plan.PKLSnippet, "20.min") {
		t.Fatalf("wrong plan: %+v", plan)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("planner edited source")
	}
}

func TestDisposableContextRejectsSymlinksAndExpandedScope(t *testing.T) {
	identity := codebase.Identity{Kind: "classic", Endpoint: "http://localhost:49684"}
	for _, kind := range []string{"source-link", "marker-link", "moved-directory", "scope", "binding"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			outside := t.TempDir()
			path := dir + "/main.json"
			if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			metadata := authoringMetadata{Version: 1, Directory: dir, Identity: identity, Stacks: []string{"owned"}}
			if kind == "moved-directory" {
				metadata.Directory = outside
			}
			raw, _ := json.Marshal(metadata)
			marker := dir + "/" + authoringMetadataName
			if err := os.WriteFile(marker, raw, 0600); err != nil {
				t.Fatal(err)
			}
			source := &tools.SourceContext{Mode: "none", TemporaryDirectory: dir}
			stacks := []string{"owned"}
			switch kind {
			case "source-link":
				if err := os.WriteFile(outside+"/main.json", []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside+"/main.json", path); err != nil {
					t.Fatal(err)
				}
			case "marker-link":
				if err := os.Rename(marker, outside+"/marker"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside+"/marker", marker); err != nil {
					t.Fatal(err)
				}
			case "scope":
				stacks = append(stacks, "other")
			case "binding":
				source.BindingID = "another-project"
			}
			if err := validateDisposable(identity, source, path, stacks); err == nil {
				t.Fatal("invalid context accepted")
			}
		})
	}
}

func TestPrepareAuthoringRetainsLargeCompleteSourceAndStrictContext(t *testing.T) {
	payload := strings.Repeat("z", 256<<10)
	forma := `{"Stacks":[{"Label":"owned","Policies":[{"Type":"ttl","TTLSeconds":3600}]}],"Targets":[{"Label":"target","Config":{"number":9007199254740993}}],"Resources":[{"Stack":"owned","Type":"Test::Object","Label":"large","Properties":{"payload":"` + payload + `","secret":{"$value":"opaque","$visibility":"secret"}}}],"Generators":[{"Stack":"owned","Type":"password","Label":"g"}],"Extraction":{"CompleteStacks":[{"Label":"owned"}],"ReferenceGenerators":[{"Stack":"external","Label":"ref"}]}}`
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, forma) },
		"GET /api/v1/plugins": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"plugins":[{"type":"resource","namespace":"test","name":"test","installedVersion":"1.2.3"}]}`)
		},
	})
	defer agent.Close()
	conn := config.Classic{URL: agent.URL}
	dir := t.TempDir()
	registry := t.TempDir() + "/registry.json"
	session := authoringSession(t, conn, agent.URL, authoringCLI(t), registry)
	var result authoringResult
	codebaseCall(t, session, "prepare_authoring", map[string]any{"temporary_directory": dir, "stacks": []string{"owned"}}, &result)
	source, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{payload, `"$visibility": "secret"`, `"ReferenceGenerators"`, `"Policies"`, `9007199254740993`} {
		if !strings.Contains(string(source), part) {
			t.Errorf("complete source lost %q", part[:min(len(part), 30)])
		}
	}
	marker, err := os.ReadFile(filepath.Join(dir, authoringMetadataName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(marker), "opaque") || strings.Contains(string(marker), "Properties") {
		t.Fatal("source properties leaked into context metadata")
	}
}

func TestPrepareAuthoringReturnsUnresolvedDesiredDiagnostics(t *testing.T) {
	diagnostics := `[{"Code":"unresolved_desired_reference","Path":"/Resources/0/Properties/producer","Reference":"formae://resource/original-producer#/name","Message":"Original producer has no eligible desired declaration"}]`
	forma := `{"Stacks":[{"Label":"owned"}],"Targets":[{"Label":"target","Config":{"number":9007199254740993}}],"Resources":[{"Stack":"owned","Label":"failed-child","Properties":{"producer":{"$ref":"formae://resource/original-producer#/name"}}}],"Extraction":{"CompleteStacks":[{"Label":"owned"}],"Diagnostics":` + diagnostics + `}}`
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, forma) },
		"GET /api/v1/plugins": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Plugins":[{"type":"resource","namespace":"test","name":"test","installedVersion":"1.2.3"}]}`)
		},
	})
	defer agent.Close()
	dir := t.TempDir()
	session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, authoringCLI(t), t.TempDir()+"/registry.json")
	var result map[string]json.RawMessage
	codebaseCall(t, session, "prepare_authoring", map[string]any{"temporary_directory": dir, "stacks": []string{"owned"}}, &result)
	var got, want any
	if err := json.Unmarshal(result["diagnostics"], &got); err != nil {
		t.Fatalf("repair diagnostics unavailable: %v", err)
	}
	if err := json.Unmarshal([]byte(diagnostics), &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("diagnostics lost original identity: %s", gotJSON)
	}
	source, err := os.ReadFile(filepath.Join(dir, "main.pkl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "original-producer") || !strings.Contains(string(source), "failed-child") {
		t.Fatal("repairable declaration was dropped before rendering")
	}
}

func TestPrepareAuthoringFreezesRoutedIdentityAcrossProfileEdit(t *testing.T) {
	ec := hostedCtx("Bearer authoring-token")
	ec.FormaeBin = authoringCLI(t)
	resolver := &stubResolver{ec: ec}
	requests := 0
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Header.Get("Formae-Installation") != testInstallation || r.Header.Get("Authorization") != "Bearer authoring-token" {
				t.Error("wrong routed installation/auth")
			}
			moved := ec
			moved.Conn = config.Hosted{Endpoint: config.HostedOrigin, Installation: "222222222222222222222222222"}
			resolver.ec = moved
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Header.Get("Formae-Installation") != testInstallation {
				t.Error("profile edit redirected desired extraction")
			}
			_, _ = fmt.Fprint(w, `{"Stacks":[{"Label":"owned"}],"Targets":[{"Label":"target","Config":{"number":9007199254740993}}],"Extraction":{"CompleteStacks":[{"Label":"owned"}]}}`)
		},
		"GET /api/v1/plugins": func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Header.Get("Formae-Installation") != testInstallation {
				t.Error("profile edit redirected plugin metadata")
			}
			_, _ = fmt.Fprint(w, `{"plugins":[{"type":"resource","namespace":"test","name":"test","installedVersion":"1.2.3"}]}`)
		},
	})
	defer agent.Close()
	s := New("")
	s.gate = func() error { return nil }
	s.ctxResolver = resolver
	s.codebaseRegistry = func() (codebase.Registry, error) { return codebase.Registry{Path: t.TempDir() + "/registry.json"}, nil }
	s.newClient = func(resolved execctx.Context) (*FormaeClient, error) {
		if resolved.Conn != ec.Conn {
			t.Error("client received edited profile")
		}
		return newTestHostedClient(t, agent, "Bearer authoring-token", nil), nil
	}
	dir := t.TempDir()
	result, _, err := s.handlePrepareAuthoring(context.Background(), nil, tools.PrepareAuthoringInput{Profile: "prod", TemporaryDirectory: dir, Stacks: []string{"owned"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(textContent(t, result))
	}
	if resolver.calls != 1 || requests != 4 {
		t.Fatalf("resolver=%d requests=%d", resolver.calls, requests)
	}
	marker, err := os.ReadFile(dir + "/" + authoringMetadataName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(marker, []byte(testInstallation)) || bytes.Contains(marker, []byte("authoring-token")) {
		t.Fatal("wrong or credential-bearing persisted identity")
	}
}

func TestFailedRenderingCannotLeaveUsableDisposableContext(t *testing.T) {
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"Stacks":[{"Label":"owned"}],"Extraction":{"CompleteStacks":[{"Label":"owned"}]}}`)
		},
		"GET /api/v1/plugins": func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{"plugins":[]}`) },
	})
	defer agent.Close()
	conn := config.Classic{URL: agent.URL}
	identity, _ := codebase.IdentityForConnection(conn)
	dir := t.TempDir()
	bin := t.TempDir() + "/formae"
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s' '{}' > main.pkl\nprintf '%s' 'amends \"pkl:Project\"' > PklProject\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s := New("")
	registry := codebase.Registry{Path: t.TempDir() + "/registry.json"}
	s.codebaseRegistry = func() (codebase.Registry, error) { return registry, nil }
	_, err := s.prepareAuthoring(context.Background(), execctx.Context{Conn: conn, FormaeBin: bin}, NewFormaeClient(agent.URL), tools.PrepareAuthoringInput{TemporaryDirectory: dir, Stacks: []string{"owned"}})
	if err == nil {
		t.Fatal("failed renderer accepted")
	}
	if err := validateDisposable(identity, &tools.SourceContext{Mode: "none", TemporaryDirectory: dir}, dir+"/main.pkl", []string{"owned"}); err == nil {
		t.Fatal("partial failed rendering left an apparently complete usable context")
	}
}

func TestDesiredStacksPreservesQueryPhraseLabels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		label string
		query string
	}{
		{"space", "with space", `stack:"with space"`},
		{"quote", `say"hello`, `stack:"say\"hello"`},
		{"backslash", `path\name`, `stack:"path\\name"`},
		{"unicode", "日本語", `stack:"日本語"`},
		{"nonbreaking_space", "x\u00a0y", "stack:\"x\u00a0y\""},
		{"line_separator", "x\u2028y", "stack:\"x\u2028y\""},
		{"figure_space", "x\u2007y", "stack:\"x\u2007y\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			labels, err := authoringStackLabels([]string{tc.label}, nil)
			if err != nil {
				t.Fatal(err)
			}
			agent := mockAgent(t, map[string]http.HandlerFunc{
				"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
					// The agent's phrase grammar unescapes quotes/backslashes; Unicode
					// characters must arrive literally, not as Go string escapes.
					if got := r.URL.Query().Get("query"); got != tc.query {
						t.Errorf("desired stack selector = %q, want %q", got, tc.query)
						http.Error(w, "different stack selector", http.StatusBadRequest)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Extraction": map[string]any{"CompleteStacks": []map[string]string{{"Label": tc.label}}}})
				},
			})
			if _, err := NewFormaeClient(agent.URL).desiredStacks(context.Background(), labels); err != nil {
				t.Fatal(err)
			}
		})
	}
}
