package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This file guards the Azure vars.pkl template documented in
// skills/formae-connect/SKILL.md's step 6 - the harness writes that template
// by hand, not this build, so nothing here runs it in production. What this
// checks is that the template, once filled in with the registration's own
// field names, actually evaluates against the real formae and azure plugin
// schemas and comes back out the other side unchanged. Inspecting the
// rendered string alone would miss a schema-generation error the same way
// checking an object built in memory would: the whole point is to catch a
// class or field renamed out from under the template before a real user does.
//
// Needs two environment variables to actually run - see
// resolveAzureTargetSchemaPaths - because the schema checkouts it evaluates
// against are private, developer-machine state that must never be written
// down as a path or a branch name in this public repo.

// azureTargetVarsSpec is what the skill's Azure vars.pkl template fills in
// from the registration and the user's own answers.
type azureTargetVarsSpec struct {
	Label          string
	SubscriptionID string
	TenantID       string
	ClientID       string
}

// azureVarsPKLPlaceholders maps each field name the SKILL.md template
// assigns a `"<...>"` placeholder to the spec value that replaces it.
var azureVarsPKLPlaceholders = map[string]func(azureTargetVarsSpec) string{
	"label":          func(s azureTargetVarsSpec) string { return s.Label },
	"subscriptionId": func(s azureTargetVarsSpec) string { return s.SubscriptionID },
	"tenantId":       func(s azureTargetVarsSpec) string { return s.TenantID },
	"clientId":       func(s azureTargetVarsSpec) string { return s.ClientID },
}

// azureVarsPKLPlaceholder matches a `field = "<...>"` assignment in the
// SKILL.md template, capturing the field name.
var azureVarsPKLPlaceholder = regexp.MustCompile(`(\w+)\s*=\s*"<[^"]*>"`)

// skillMDPath is relative to this package's directory, which is also `go
// test`'s working directory.
const skillMDPath = "../../skills/formae-connect/SKILL.md"

// azureVarsPKLMarker is the prose immediately before the Azure vars.pkl
// fenced block in SKILL.md's step 6. If this string stops matching, the
// section was edited or renamed and this test's marker needs the same
// update - a silent match against the wrong fence would defeat the point of
// reading the shipped artifact instead of a hand copy.
const azureVarsPKLMarker = "**Azure** declares the same target"

// renderAzureTargetVarsPKL reads the Azure vars.pkl example straight out of
// skills/formae-connect/SKILL.md and substitutes spec's values for its
// `"<...>"` placeholders. This is the artifact the harness actually writes,
// so this test guards against the SKILL.md text drifting from the real
// schema - a hand-maintained copy in Go would instead let SKILL.md drift
// while the test kept passing against the stale copy.
func renderAzureTargetVarsPKL(t *testing.T, spec azureTargetVarsSpec) string {
	t.Helper()

	src, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read %s: %v", skillMDPath, err)
	}

	markerIdx := strings.Index(string(src), azureVarsPKLMarker)
	if markerIdx == -1 {
		t.Fatalf("%s no longer contains %q; update this test's marker to match wherever the Azure vars.pkl example moved to",
			skillMDPath, azureVarsPKLMarker)
	}
	rest := string(src)[markerIdx:]

	const fence = "```pkl\n"
	fenceStart := strings.Index(rest, fence)
	if fenceStart == -1 {
		t.Fatalf("no ```pkl fence found after %q in %s", azureVarsPKLMarker, skillMDPath)
	}
	rest = rest[fenceStart+len(fence):]

	fenceEnd := strings.Index(rest, "```")
	if fenceEnd == -1 {
		t.Fatalf("unterminated ```pkl fence after %q in %s", azureVarsPKLMarker, skillMDPath)
	}
	block := rest[:fenceEnd]

	return azureVarsPKLPlaceholder.ReplaceAllStringFunc(block, func(m string) string {
		field := azureVarsPKLPlaceholder.FindStringSubmatch(m)[1]
		fill, ok := azureVarsPKLPlaceholders[field]
		if !ok {
			t.Fatalf("%s's Azure vars.pkl example assigns a placeholder to unrecognized field %q; "+
				"add it to azureVarsPKLPlaceholders", skillMDPath, field)
		}
		return fmt.Sprintf("%s = %q", field, fill(spec))
	})
}

// Environment variables resolveAzureTargetSchemaPaths reads. Named rather
// than hardcoded paths because this repository is public: a private
// directory convention (this org's own dev machines keep everything under
// ~/dev/pel) and an internal branch name (formae-plugin-azure's oidc-auth,
// ahead of its merge to that plugin's main) are exactly the kind of thing
// that must never be written down in it.
const (
	envFormaeSchemaPklProject = "FORMAE_SCHEMA_PKL_PROJECT"
	envAzureSchemaPkl         = "FORMAE_AZURE_SCHEMA_PKL"
)

// resolveAzureTargetSchemaPaths reads the local schema checkouts this test
// evaluates the rendered template against.
//
// Skips only when a variable is unset: a machine with neither checkout
// configured (most CI runners today) cannot run the real schema check, and
// that is an environmental precondition, not a test failure. Once a variable
// IS set, a path that does not work is a hard failure rather than a skip - a
// silent skip there would mean a green run stops proving anything the day the
// path quietly breaks, and nobody would hear about it.
func resolveAzureTargetSchemaPaths(t *testing.T) (formaePklProject, azurePkl string) {
	t.Helper()

	if _, err := exec.LookPath("pkl"); err != nil {
		t.Skip("pkl binary not found on PATH; skipping the real schema round-trip check")
	}

	formaePklProject = os.Getenv(envFormaeSchemaPklProject)
	azurePkl = os.Getenv(envAzureSchemaPkl)
	if formaePklProject == "" || azurePkl == "" {
		t.Skipf("%s and %s must both be set to run the real schema round-trip check "+
			"(%s -> formae's schema/pkl/schema/PklProject, %s -> formae-plugin-azure's schema/pkl/azure.pkl "+
			"with the OidcAuth block); skipping",
			envFormaeSchemaPklProject, envAzureSchemaPkl, envFormaeSchemaPklProject, envAzureSchemaPkl)
	}

	if _, err := os.Stat(formaePklProject); err != nil {
		t.Fatalf("%s=%s is not usable: %v", envFormaeSchemaPklProject, formaePklProject, err)
	}
	azureSrc, err := os.ReadFile(azurePkl)
	if err != nil {
		t.Fatalf("%s=%s is not usable: %v", envAzureSchemaPkl, azurePkl, err)
	}
	if !strings.Contains(string(azureSrc), "OidcAuth") {
		t.Fatalf("%s=%s does not declare OidcAuth; point it at the schema variant that carries it",
			envAzureSchemaPkl, azurePkl)
	}

	return formaePklProject, azurePkl
}

// azureTargetEvalDoc is the shape pkl eval -f json produces for the rendered
// vars.pkl, matching formae.Target's own Fixed output fields.
type azureTargetEvalDoc struct {
	AzureTarget struct {
		Label  string `json:"Label"`
		Config struct {
			Type           string `json:"Type"`
			SubscriptionID string `json:"SubscriptionId"`
			Auth           struct {
				Type     string `json:"Type"`
				TenantID string `json:"TenantId"`
				ClientID string `json:"ClientId"`
			} `json:"Auth"`
		} `json:"Config"`
		Discoverable bool `json:"Discoverable"`
	} `json:"azureTarget"`
}

// pklEvalAzureTarget writes the rendered vars.pkl into a project set up to
// resolve @formae/formae.pkl and @azure/azure.pkl entirely from local
// checkouts - no network, no published package - and evaluates it for real.
//
// azure.pkl is copied rather than referenced in place: Pkl resolves a
// module's imports against the PklProject governing its own directory by
// ancestry, not the directory pkl is invoked from, so the only way to make
// @formae/formae.pkl resolve locally for a schema file this test does not
// own is to place a copy of it under a project declaring that dependency.
func pklEvalAzureTarget(t *testing.T, spec azureTargetVarsSpec) azureTargetEvalDoc {
	t.Helper()
	formaePklProject, azurePkl := resolveAzureTargetSchemaPaths(t)

	dir := t.TempDir()
	azurePkgDir := filepath.Join(dir, "azure-pkg")
	if err := os.Mkdir(azurePkgDir, 0o755); err != nil {
		t.Fatalf("mkdir azure-pkg: %v", err)
	}

	azureSrc, err := os.ReadFile(azurePkl)
	if err != nil {
		t.Fatalf("read %s: %v", azurePkl, err)
	}
	if err := os.WriteFile(filepath.Join(azurePkgDir, "azure.pkl"), azureSrc, 0o644); err != nil {
		t.Fatalf("write azure.pkl: %v", err)
	}
	azurePklProject := fmt.Sprintf("amends \"pkl:Project\"\n\ndependencies {\n  [\"formae\"] = import(%q)\n}\n\n"+
		"package {\n  name = \"azure\"\n  baseUri = \"package://test.local/azure\"\n  version = \"0.0.0\"\n"+
		"  packageZipUrl = \"https://test.local/azure@0.0.0.zip\"\n}\n", formaePklProject)
	if err := os.WriteFile(filepath.Join(azurePkgDir, "PklProject"), []byte(azurePklProject), 0o644); err != nil {
		t.Fatalf("write azure-pkg/PklProject: %v", err)
	}

	pklProject := fmt.Sprintf("amends \"pkl:Project\"\n\ndependencies {\n  [\"formae\"] = import(%q)\n  [\"azure\"] = import(%q)\n}\n",
		formaePklProject, filepath.Join(azurePkgDir, "PklProject"))
	if err := os.WriteFile(filepath.Join(dir, "PklProject"), []byte(pklProject), 0o644); err != nil {
		t.Fatalf("write PklProject: %v", err)
	}

	varsPath := filepath.Join(dir, "vars.pkl")
	if err := os.WriteFile(varsPath, []byte(renderAzureTargetVarsPKL(t, spec)), 0o644); err != nil {
		t.Fatalf("write vars.pkl: %v", err)
	}

	// --project-dir explicitly on both commands: Pkl resolves an `@alias`
	// import's project by walking up from the invoking process's working
	// directory, not from the module file's own path, so leaving it implicit
	// makes resolution depend on the test binary's cwd rather than this dir.
	resolve := exec.Command("pkl", "project", "resolve", dir)
	if out, err := resolve.CombinedOutput(); err != nil {
		t.Fatalf("pkl project resolve: %v\n%s", err, out)
	}

	out, err := exec.Command("pkl", "eval", "--project-dir", dir, "-f", "json", varsPath).CombinedOutput()
	if err != nil {
		t.Fatalf("pkl eval: %v\n%s", err, out)
	}

	var doc azureTargetEvalDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("decode evaluated json: %v\n%s", err, out)
	}
	return doc
}

// TestAzureTargetRoundTrips asserts the registration's azureTenantId and
// azureClientId become the OidcAuth block while the subscription id remains
// the account identifier, and that the rendered file evaluates against the
// real schema and deserializes back to the same variant.
func TestAzureTargetRoundTrips(t *testing.T) {
	spec := azureTargetVarsSpec{
		Label:          "azure-test",
		SubscriptionID: "11111111-1111-1111-1111-111111111111",
		TenantID:       "22222222-2222-2222-2222-222222222222",
		ClientID:       "33333333-3333-3333-3333-333333333333",
	}

	doc := pklEvalAzureTarget(t, spec)

	if doc.AzureTarget.Label != spec.Label {
		t.Errorf("Label: got %q, want %q", doc.AzureTarget.Label, spec.Label)
	}
	if doc.AzureTarget.Config.Type != "Azure" {
		t.Errorf("Config.Type: got %q, want \"Azure\"", doc.AzureTarget.Config.Type)
	}
	if doc.AzureTarget.Config.SubscriptionID != spec.SubscriptionID {
		t.Errorf("Config.SubscriptionId: got %q, want %q", doc.AzureTarget.Config.SubscriptionID, spec.SubscriptionID)
	}
	if doc.AzureTarget.Config.Auth.Type != "Oidc" {
		t.Errorf("Config.Auth.Type: got %q, want \"Oidc\"", doc.AzureTarget.Config.Auth.Type)
	}
	if doc.AzureTarget.Config.Auth.TenantID != spec.TenantID {
		t.Errorf("Config.Auth.TenantId: got %q, want %q", doc.AzureTarget.Config.Auth.TenantID, spec.TenantID)
	}
	if doc.AzureTarget.Config.Auth.ClientID != spec.ClientID {
		t.Errorf("Config.Auth.ClientId: got %q, want %q", doc.AzureTarget.Config.Auth.ClientID, spec.ClientID)
	}
	if !doc.AzureTarget.Discoverable {
		t.Error("Discoverable: got false, want true - a target that discovers nothing produces no error to explain itself")
	}
}

// TestRenderAzureTargetVarsPKL_FillsEveryPlaceholderFromTheRealSkillDoc
// guards renderAzureTargetVarsPKL's extraction on its own, independent of
// pkl being installed: it must read SKILL.md's actual Azure vars.pkl example
// and leave no placeholder unfilled.
func TestRenderAzureTargetVarsPKL_FillsEveryPlaceholderFromTheRealSkillDoc(t *testing.T) {
	spec := azureTargetVarsSpec{
		Label:          "my-label",
		SubscriptionID: "sub-id",
		TenantID:       "tenant-id",
		ClientID:       "client-id",
	}

	got := renderAzureTargetVarsPKL(t, spec)

	for _, want := range []string{
		`import "@formae/formae.pkl"`,
		`import "@azure/azure.pkl"`,
		`label = "my-label"`,
		`subscriptionId = "sub-id"`,
		`tenantId = "tenant-id"`,
		`clientId = "client-id"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered template does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<") {
		t.Errorf("a placeholder was left unfilled:\n%s", got)
	}
}
