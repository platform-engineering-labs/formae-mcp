// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

func TestCodebaseContextToolIsLocalAndExplicit(t *testing.T) {
	// No agent is running here: context selection only reads local registration.
	session := codebaseTestSession(t, config.Classic{URL: "http://localhost", Port: 49684}, filepath.Join(t.TempDir(), "registry.json"))
	var initial codebaseContextResult
	codebaseCall(t, session, "get_codebase_context", map[string]any{}, &initial)
	if initial.Selection.Mode != codebase.ModeUnconfigured {
		t.Fatalf("fresh classic context: %+v", initial)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_codebase_context", Arguments: map[string]any{"mode": "none"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(textContent(t, result))
	}
	var got struct {
		Selection struct {
			Mode string `json:"mode"`
		} `json:"selection"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &got); err != nil {
		t.Fatal(err)
	}
	if got.Selection.Mode != "none" {
		t.Fatalf("selection = %q, want explicit none", got.Selection.Mode)
	}
}

func TestDriftPreferenceToolPersistsExplicitChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	conn := config.Classic{URL: "http://localhost", Port: 49684}
	session := codebaseTestSession(t, conn, path)
	var initial map[string]any
	codebaseCall(t, session, "get_codebase_context", map[string]any{}, &initial)
	if initial["drift_preference"] == nil {
		t.Fatal("context omitted preference")
	}
	var saved map[string]any
	codebaseCall(t, session, "set_drift_preference", map[string]any{"mode": "auto_absorb_external"}, &saved)
	restarted := codebaseTestSession(t, conn, path)
	codebaseCall(t, restarted, "get_codebase_context", map[string]any{}, &initial)
	p := initial["drift_preference"].(map[string]any)
	if p["mode"] != "auto_absorb_external" || p["explicit"] != true {
		t.Fatalf("preference: %#v", p)
	}
}

func codebaseTestSession(t *testing.T, conn config.Connection, registryPath string) *mcp.ClientSession {
	t.Helper()
	s := New("")
	s.clientID = testClientIDResolver(t)
	s.gate = func() error { return nil }
	s.ctxResolver = codebaseFixedResolver{conn: conn}
	s.codebaseRegistry = func() (codebase.Registry, error) { return codebase.Registry{Path: registryPath}, nil }
	a, b := mcp.NewInMemoryTransports()
	serverSession, err := s.mcpServer.Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "context-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// This resolver is immutable so concurrent MCP calls exercise the production
// handlers without introducing a race in test instrumentation.
type codebaseFixedResolver struct{ conn config.Connection }

func (r codebaseFixedResolver) Resolve(_ context.Context, profile string, _ bool) (execctx.Context, error) {
	return execctx.Context{ProfileName: profile, Conn: r.conn}, nil
}
func (codebaseFixedResolver) Bin() string   { return "formae" }
func (codebaseFixedResolver) Managed() bool { return false }

func codebaseCall(t *testing.T, session *mcp.ClientSession, name string, input map[string]any, output any) {
	t.Helper()
	r, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatal(textContent(t, r))
	}
	if output != nil {
		reflect.ValueOf(output).Elem().SetZero()
		if err := json.Unmarshal([]byte(textContent(t, r)), output); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCodebaseContextConcurrentCallsSelectIndependently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	session := codebaseTestSession(t, config.Classic{URL: "http://localhost", Port: 49684}, path)
	var first, second codebase.Binding
	codebaseCall(t, session, "register_codebase", map[string]any{"path": contextProject(t)}, &first)
	codebaseCall(t, session, "register_codebase", map[string]any{"path": contextProject(t)}, &second)
	type reply struct {
		index  int
		result *mcp.CallToolResult
		err    error
	}
	replies := make(chan reply, 12)
	for i := 0; i < 12; i++ {
		go func(index int) {
			input := map[string]any{"binding_id": first.ID}
			if index%3 == 1 {
				input["binding_id"] = second.ID
			}
			if index%3 == 2 {
				input = map[string]any{"mode": "none"}
			}
			r, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_codebase_context", Arguments: input})
			replies <- reply{index: index, result: r, err: e}
		}(i)
	}
	for i := 0; i < 12; i++ {
		r := <-replies
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.result.IsError {
			t.Fatal(textContent(t, r.result))
		}
		var got codebaseContextResult
		if err := json.Unmarshal([]byte(textContent(t, r.result)), &got); err != nil {
			t.Fatal(err)
		}
		if r.index%3 == 2 {
			if got.Selection.Mode != codebase.ModeNone || got.Selection.Binding != nil {
				t.Fatalf("none call inherited a project: %+v", got)
			}
			continue
		}
		want := first.ID
		if r.index%3 == 1 {
			want = second.ID
		}
		if got.Selection.Binding == nil || got.Selection.Binding.ID != want {
			t.Fatalf("call %d selected %+v, want %s", r.index, got, want)
		}
	}
}

func codebaseCallError(t *testing.T, session *mcp.ClientSession, name string, input map[string]any, want string) {
	t.Helper()
	r, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(textContent(t, r), want) {
		t.Fatalf("result error=%v content=%s; want %q", r.IsError, textContent(t, r), want)
	}
}

func contextProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PklProject"), []byte("amends \"pkl:Project\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCodebaseToolsPersistSelectionWithoutSessionActiveProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	conn := config.Hosted{Endpoint: config.HostedOrigin, Installation: testInstallation}
	a := codebaseTestSession(t, conn, path)
	var contextResult codebaseContextResult
	codebaseCall(t, a, "get_codebase_context", map[string]any{}, &contextResult)
	if contextResult.Selection.Mode != codebase.ModeNone {
		t.Fatalf("fresh hosted context: %+v", contextResult)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discovery created registry: %v", err)
	}

	first, second := contextProject(t), contextProject(t)
	var one, two, repeated codebase.Binding
	codebaseCall(t, a, "register_codebase", map[string]any{"path": first, "stacks": []string{"app"}}, &one)
	codebaseCall(t, a, "register_codebase", map[string]any{"path": second, "stacks": []string{"ops"}}, &two)
	codebaseCall(t, a, "register_codebase", map[string]any{"path": first, "stacks": []string{"app"}}, &repeated)
	if one.ID == two.ID || one.ID != repeated.ID {
		t.Fatal("binding identities not stable and distinct")
	}
	for _, dir := range []string{first, second} {
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 1 || files[0].Name() != "PklProject" {
			t.Fatalf("registration changed project: %v %v", files, err)
		}
	}
	// A fresh MCP instance gets the same registry, independent of its last call.
	b := codebaseTestSession(t, conn, path)
	codebaseCall(t, b, "get_codebase_context", map[string]any{}, &contextResult)
	if contextResult.Selection.Mode != codebase.ModeSelect || len(contextResult.Selection.Candidates) != 2 {
		t.Fatalf("missing persisted candidates: %+v", contextResult)
	}
	codebaseCall(t, a, "get_codebase_context", map[string]any{"binding_id": one.ID, "stacks": []string{"app"}}, &contextResult)
	if contextResult.Selection.Binding.ID != one.ID {
		t.Fatal("wrong selected binding")
	}
	codebaseCall(t, b, "get_codebase_context", map[string]any{"working_directory": second, "stacks": []string{"ops"}}, &contextResult)
	if contextResult.Selection.Binding.ID != two.ID {
		t.Fatal("workspace selected wrong project")
	}
	codebaseCall(t, a, "get_codebase_context", map[string]any{"binding_id": one.ID}, &contextResult)
	if contextResult.Selection.Binding.ID != one.ID {
		t.Fatal("another session changed selection")
	}
	codebaseCallError(t, a, "get_codebase_context", map[string]any{"binding_id": one.ID, "stacks": []string{"ops"}}, "outside")
	codebaseCall(t, a, "get_codebase_context", map[string]any{"mode": "none"}, &contextResult)
	if contextResult.Selection.Mode != codebase.ModeNone {
		t.Fatal("explicit none did not win")
	}
	var listing codebaseListResult
	codebaseCall(t, b, "list_codebases", map[string]any{}, &listing)
	if len(listing.Bindings) != 2 || listing.Identity.Installation != testInstallation {
		t.Fatalf("wrong installation listing: %+v", listing)
	}
}

func TestCodebaseToolsRefuseWrongInstallationAndKeepMissingProjectsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	conn := config.Hosted{Endpoint: config.HostedOrigin, Installation: testInstallation}
	a := codebaseTestSession(t, conn, path)
	b := codebaseTestSession(t, config.Hosted{Endpoint: config.HostedOrigin, Installation: "4" + testInstallation[1:]}, path)
	dir := contextProject(t)
	var binding codebase.Binding
	codebaseCall(t, a, "register_codebase", map[string]any{"path": dir}, &binding)
	codebaseCallError(t, b, "get_codebase_context", map[string]any{"binding_id": binding.ID}, "another installation")
	codebaseCallError(t, b, "unregister_codebase", map[string]any{"binding_id": binding.ID}, "another installation")
	var listing codebaseListResult
	codebaseCall(t, b, "list_codebases", map[string]any{}, &listing)
	if len(listing.Bindings) != 0 {
		t.Fatal("other installation leaked into listing")
	}
	if err := os.Remove(filepath.Join(dir, "PklProject")); err != nil {
		t.Fatal(err)
	}
	codebaseCall(t, a, "list_codebases", map[string]any{}, &listing)
	if len(listing.Bindings) != 1 || listing.Bindings[0].Available || listing.Bindings[0].Problem == "" {
		t.Fatalf("missing project hidden: %+v", listing)
	}
	codebaseCallError(t, a, "get_codebase_context", map[string]any{"binding_id": binding.ID}, "unavailable")
	codebaseCall(t, a, "unregister_codebase", map[string]any{"binding_id": binding.ID}, nil)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("unregistration removed directory: %v", err)
	}
	codebaseCall(t, a, "list_codebases", map[string]any{}, &listing)
	if len(listing.Bindings) != 0 {
		t.Fatal("binding not removed")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	codebaseCallError(t, a, "get_codebase_context", map[string]any{"mode": "none"}, "registry")
}
