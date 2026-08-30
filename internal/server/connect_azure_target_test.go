package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// azureTargetVarsSpec is what the skill's Azure vars.pkl template fills in
// from the registration and the user's own answers.
type azureTargetVarsSpec struct {
	Label          string
	SubscriptionID string
	TenantID       string
	ClientID       string
}

// renderAzureTargetVarsPKL renders the vars.pkl content the skill instructs
// the harness to write for an Azure target. Keep this byte-for-byte in step
// with the Azure vars.pkl example in skills/formae-connect/SKILL.md's step 6
// - if one changes, so must the other.
func renderAzureTargetVarsPKL(spec azureTargetVarsSpec) string {
	return fmt.Sprintf(`import "@formae/formae.pkl"
import "@azure/azure.pkl"

azureTarget: formae.Target = new formae.Target {
  label = %q
  discoverable = true
  config = new azure.Config {
    subscriptionId = %q
    auth = new azure.OidcAuth {
      tenantId = %q
      clientId = %q
    }
  }
}
`, spec.Label, spec.SubscriptionID, spec.TenantID, spec.ClientID)
}

// findAzureTargetSchemaCheckouts locates the local formae and formae-plugin-
// azure checkouts this test evaluates the rendered template against,
// following this org's documented "everything under ~/dev/pel" repo
// topology. It skips rather than fails when they are not found: a machine
// without both checkouts (most CI runners) cannot run the real schema check,
// and that is an environmental precondition, not a test failure.
//
// The azure.pkl candidate list tries the plain checkout first, then the
// oidc-auth branch worktree: once that branch's OidcAuth block reaches
// formae-plugin-azure's main branch, the plain checkout picks it up with no
// change needed here.
func findAzureTargetSchemaCheckouts(t *testing.T) (formaePklProject, azurePkl string) {
	t.Helper()

	if _, err := exec.LookPath("pkl"); err != nil {
		t.Skip("pkl binary not found on PATH; skipping the real schema round-trip check")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot resolve the home directory to locate local schema checkouts")
	}

	formaePklProject = filepath.Join(home, "dev/pel/formae/internal/schema/pkl/schema/PklProject")
	if _, err := os.Stat(formaePklProject); err != nil {
		t.Skipf("no formae schema checkout at %s; skipping the real schema round-trip check", formaePklProject)
	}

	for _, candidate := range []string{
		filepath.Join(home, "dev/pel/formae-plugin-azure/schema/pkl/azure.pkl"),
		filepath.Join(home, "dev/pel/formae-plugin-azure/.worktrees/oidc-auth/schema/pkl/azure.pkl"),
	} {
		src, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if strings.Contains(string(src), "OidcAuth") {
			return formaePklProject, candidate
		}
	}
	t.Skip("no local formae-plugin-azure checkout with the OidcAuth schema was found; " +
		"skipping the real schema round-trip check")
	return "", ""
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
	formaePklProject, azurePkl := findAzureTargetSchemaCheckouts(t)

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
	if err := os.WriteFile(varsPath, []byte(renderAzureTargetVarsPKL(spec)), 0o644); err != nil {
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
