package server

import (
	"strings"
	"testing"
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
