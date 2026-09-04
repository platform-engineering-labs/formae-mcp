package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// A -dev build publishes its schema package over the canonical X.Y.Z
// coordinate, so that is the version a project pins. No other prerelease
// carries that convention, and naming one would point at a different
// published version.
func TestCanonicalSchemaVersion(t *testing.T) {
	cases := []struct {
		installed string
		want      string
		wantOK    bool
	}{
		{"0.1.10", "0.1.10", true},
		{"0.1.17-dev.8", "0.1.17", true},
		{"0.1.17-dev", "0.1.17", true},
		{"1.2.3-rc.1", "", false},
		{"1.2.3-beta.2", "", false},
		{"1.2.3-dev.4.5", "", false},
		{"0.1", "", false},
		{"latest", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := canonicalSchemaVersion(tc.installed)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("canonicalSchemaVersion(%q) = (%q, %v), want (%q, %v)",
				tc.installed, got, ok, tc.want, tc.wantOK)
		}
	}
}

func testPlugins() []agentPlugin {
	return []agentPlugin{
		{Name: "aws", Kind: "plugin", Type: "resource", InstalledVersion: "0.1.17-dev.8"},
		{Name: "azure", Kind: "plugin", Type: "resource", InstalledVersion: "0.1.10"},
		{Name: "auth-basic", Kind: "plugin", Type: "auth", InstalledVersion: "0.1.0"},
		{Name: "standard", Kind: "metapackage", Type: "resource", InstalledVersion: "0.2.0"},
	}
}

// section returns the text between two headings, failing with which heading is
// missing rather than panicking on a negative slice index.
func section(t *testing.T, out, from, to string) string {
	t.Helper()
	i := strings.Index(out, from)
	if i < 0 {
		t.Fatalf("heading %q missing from:\n%s", from, out)
	}
	j := strings.Index(out, to)
	if j < i {
		t.Fatalf("heading %q missing after %q in:\n%s", to, from, out)
	}
	return out[i:j]
}

// A bundle is installed on the agent but is not something to author against.
// Offering it as a plugin would send a model looking for resource types it does
// not have.
func TestRenderAgentPluginsSeparatesKinds(t *testing.T) {
	out := renderAgentPlugins(true, testPlugins())

	authorable := section(t, out, "Authorable", "Auth plugins")
	if !strings.Contains(authorable, "aws") || !strings.Contains(authorable, "azure") {
		t.Errorf("resource plugins missing from the authorable section:\n%s", out)
	}
	if strings.Contains(authorable, "standard") || strings.Contains(authorable, "auth-basic") {
		t.Errorf("a bundle or an auth plugin reached the authorable section:\n%s", out)
	}
	if !strings.Contains(out, "auth-basic") || !strings.Contains(out, "standard 0.2.0") {
		t.Errorf("auth plugins and bundles should still be reported:\n%s", out)
	}
}

func TestRenderAgentPluginsNamesTheSchemaCoordinate(t *testing.T) {
	out := renderAgentPlugins(true, testPlugins())
	if !strings.Contains(out, "aws@0.1.17") {
		t.Errorf("a -dev build should pin the canonical coordinate:\n%s", out)
	}
	if strings.Contains(out, "aws@0.1.17-dev.8") {
		t.Errorf("the -dev coordinate has never been published and must not be offered:\n%s", out)
	}
	if !strings.Contains(out, "0.1.17-dev.8") {
		t.Errorf("the installed version should still be reported:\n%s", out)
	}
}

func TestRenderAgentPluginsOmitsAnUnnameableCoordinate(t *testing.T) {
	out := renderAgentPlugins(true, []agentPlugin{
		{Name: "tailscale", Kind: "plugin", Type: "resource", InstalledVersion: "0.3.0-rc.1"},
	})
	if strings.Contains(out, "tailscale@0.3.0") {
		t.Errorf("an rc build must not be rewritten to a release coordinate:\n%s", out)
	}
	if !strings.Contains(out, "no schema version can be named") {
		t.Errorf("the omission should be stated:\n%s", out)
	}
}

func TestRenderAgentPluginsStatesTheMode(t *testing.T) {
	hosted := renderAgentPlugins(true, testPlugins())
	if !strings.Contains(hosted, "cannot be installed on demand") {
		t.Errorf("hosted rendering should say the set is closed:\n%s", hosted)
	}
	if strings.Contains(hosted, "search_hub_plugins") {
		t.Errorf("hosted rendering must not point at the hub catalogue:\n%s", hosted)
	}

	classic := renderAgentPlugins(false, testPlugins())
	if !strings.Contains(classic, "search_hub_plugins") {
		t.Errorf("classic rendering should point at the hub catalogue:\n%s", classic)
	}
	if strings.Contains(classic, "cannot be installed on demand") {
		t.Errorf("classic rendering must not claim the set is closed:\n%s", classic)
	}
}

// A hosted installation with no plugins is not a real state, so an empty
// listing means the answer could not be established. Reading it as an empty set
// would refuse every intent the user has.
func TestRenderAgentPluginsHostedEmptyIsUndetermined(t *testing.T) {
	out := renderAgentPlugins(true, nil)
	if !strings.Contains(out, "could not be established") {
		t.Errorf("an empty hosted listing should read as undetermined:\n%s", out)
	}
}

// Auth plugins and bundles are entries. An installation that reported them
// reported something, so the answer is "nothing authorable", not "nothing", and
// what it did report still has to appear.
func TestRenderAgentPluginsWithNothingAuthorable(t *testing.T) {
	out := renderAgentPlugins(true, []agentPlugin{
		{Name: "auth-basic", Kind: "plugin", Type: "auth", InstalledVersion: "0.1.0"},
		{Name: "standard", Kind: "metapackage", Type: "resource", InstalledVersion: "0.2.0"},
	})
	if strings.Contains(out, "reported nothing at all") {
		t.Errorf("entries were reported, so this is not the nothing-reported case:\n%s", out)
	}
	if !strings.Contains(out, "auth-basic") || !strings.Contains(out, "standard") {
		t.Errorf("what was reported must still be shown:\n%s", out)
	}
	if !strings.Contains(out, "undetermined") {
		t.Errorf("hosted with nothing authorable is undetermined, not empty:\n%s", out)
	}
}

// An entry whose kind or type this server does not recognise must not become a
// schema pin a user acts on.
func TestRenderAgentPluginsDoesNotInventAuthorability(t *testing.T) {
	out := renderAgentPlugins(false, []agentPlugin{
		{Name: "aws", Kind: "plugin", Type: "resource", InstalledVersion: "0.1.16"},
		{Name: "mystery", Kind: "plugin", Type: "something-new", InstalledVersion: "0.1.0"},
	})
	authorable := section(t, out, "Authorable", "Reported with a kind or type")
	if strings.Contains(authorable, "mystery") {
		t.Errorf("an unrecognised type must not render as authorable:\n%s", out)
	}
	if !strings.Contains(out, "mystery") {
		t.Errorf("an unrecognised entry must still be reported:\n%s", out)
	}
}

// The agent's order comes from a registry merged with a filesystem scan, so the
// rendering sorts rather than passing that through.
func TestRenderAgentPluginsSortsByName(t *testing.T) {
	out := renderAgentPlugins(false, []agentPlugin{
		{Name: "k8s", Kind: "plugin", Type: "resource", InstalledVersion: "0.1.10"},
		{Name: "aws", Kind: "plugin", Type: "resource", InstalledVersion: "0.1.16"},
	})
	if strings.Index(out, "aws") > strings.Index(out, "k8s") {
		t.Errorf("authorable plugins should be sorted by name:\n%s", out)
	}
}

// A self-hosted agent really can have no plugins installed yet.
func TestRenderAgentPluginsClassicEmptyIsAFact(t *testing.T) {
	out := renderAgentPlugins(false, nil)
	if strings.Contains(out, "could not be established") {
		t.Errorf("an empty classic listing is a real state, not an unreadable one:\n%s", out)
	}
	if !strings.Contains(out, "no plugins") {
		t.Errorf("an empty classic listing should say so plainly:\n%s", out)
	}
}

// The hosted branch fails closed: a listing that could not be read is not an
// empty set, and a caller that treated it as one would refuse work the
// installation can do.
func TestHandleListAgentPluginsHostedFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, _ := hostedServerFor(t, srv)
	res, _, err := s.handleListAgentPlugins(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a hosted listing failure must be an error result, got: %v", blocks(t, res))
	}
}

// A self-hosted conversation loses one fact and nothing else: the hub is still
// the catalogue and authoring needs only schemas, so the call reports the gap
// rather than failing.
func TestHandleListAgentPluginsClassicCarriesOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := serverWithStubResolver(t, execctx.Context{
		ProfileName: "local",
		Conn:        config.Classic{URL: srv.URL},
		FormaeBin:   "/usr/bin/formae",
	})
	s.newClient = func(execctx.Context) (*FormaeClient, error) {
		return newTestFormaeClient(srv), nil
	}

	res, _, err := s.handleListAgentPlugins(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("a classic listing failure must not be an error result: %v", blocks(t, res))
	}
	text := strings.Join(blocks(t, res), "\n")
	if !strings.Contains(text, "could not be read") || !strings.Contains(text, "search_hub_plugins") {
		t.Errorf("classic should say the listing could not be read and still point at the "+
			"catalogue:\n%s", text)
	}
}

// A handler test proves the branch but not that anyone can reach it: it would
// pass with the tool unregistered, misnamed, or wired to another description.
func TestListAgentPluginsIsRegistered(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	var tool *mcp.Tool
	for _, candidate := range result.Tools {
		if candidate.Name == "list_agent_plugins" {
			tool = candidate
			break
		}
	}
	if tool == nil {
		t.Fatal("list_agent_plugins is not registered")
	}
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Errorf("list_agent_plugins must be annotated read-only: %+v", tool.Annotations)
	}
	schema, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshalling the input schema: %v", err)
	}
	if !strings.Contains(string(schema), `"profile"`) {
		t.Errorf("list_agent_plugins must accept a per-call profile: %s", schema)
	}
}

// The two branches must not drift into the same wording: a successful hosted
// call carries the closed-set framing and the canonical schema coordinate.
func TestHandleListAgentPluginsHostedRendersTheClosedSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[{"name":"aws","kind":"plugin","type":"resource","installedVersion":"0.1.17-dev.8"}]}`))
	}))
	defer srv.Close()

	s, _ := hostedServerFor(t, srv)
	res, _, err := s.handleListAgentPlugins(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text := strings.Join(blocks(t, res), "\n")
	if !strings.Contains(text, "cannot be installed on demand") || !strings.Contains(text, "aws@0.1.17") {
		t.Errorf("hosted rendering missing from the result:\n%s", text)
	}
}
