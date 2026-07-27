package server

import (
	"fmt"
	"strings"
)

// docsBaseURL is the SINGLE source of truth for the documentation site's base.
// It is the only place the host (and any version/path prefix) is written. The
// docs now live on Mintlify at the site root; the old readthedocs tree used a
// "/en/latest/" version segment here. If the docs move again, change this
// constant and the relative Paths in docPages — nothing else.
// TestDocURLsShareBase enforces that every emitted doc URL is rooted here, so
// an incomplete migration fails the build.
const docsBaseURL = "https://docs.formae.io/"

// docURL builds a full documentation URL from a docsBaseURL-relative path.
func docURL(path string) string { return docsBaseURL + path }

// docPage is a canonical documentation page advertised to AI assistants via the
// formae://docs/index resource, so they use real URLs instead of constructing
// (and frequently mis-constructing) them. Path is relative to docsBaseURL.
type docPage struct {
	Title string
	Path  string
	Desc  string
}

// docPages is the curated set of dev/agent-relevant documentation pages. Paths
// are kept relative so a docs-site migration is a single-constant change. Each
// path is verified to resolve on the live site.
var docPages = []docPage{
	{"Core concepts", "documentation/concepts/forma", "Overview of stacks, targets, resources, formas, modes, drift, discovery."},
	{"Labels and renaming", "documentation/concepts/label", "Resource/stack labels, discovery-assigned labels, and renaming a resource via alias."},
	{"Stacks", "documentation/concepts/stack", "Logical grouping of resources with referential integrity."},
	{"Targets", "documentation/concepts/target", "Cloud accounts/regions where resources are deployed."},
	{"Forma files", "documentation/concepts/forma", "The infrastructure declaration unit."},
	{"Properties", "documentation/concepts/properties", "Resource properties and cross-resource references."},
	{"Resources", "documentation/concepts/resources", "Managed and unmanaged cloud resources."},
	{"Values", "documentation/concepts/values", "Resolvables and late-bound values."},
	{"Apply modes", "documentation/concepts/apply-modes", "Reconcile vs patch semantics."},
	{"Synchronization", "documentation/concepts/synchronization", "How the agent keeps cloud state in sync."},
	{"Discovery", "documentation/concepts/discovery", "Finding unmanaged resources for import."},
	{"Plugins (concept)", "documentation/concepts/plugin", "How the plugin architecture works."},
	{"PKL cheatsheet", "documentation/reference/pkl-cheatsheet", "Minimal PKL syntax for reading/writing forma files."},
	{"AI coding assistants", "documentation/guides/ai-coding-assistants", "Setting up the formae MCP with AI coding assistants."},
	{"Plugin SDK tutorial", "plugin-development/tutorial", "Build a plugin from scratch, end to end."},
	{"Plugin SDK tutorial — Scaffold", "plugin-development/tutorial/01-scaffold", "Initialize the plugin project structure."},
	{"Plugin SDK tutorial — Schema", "plugin-development/tutorial/02-schema", "Define resource types in PKL."},
	{"Plugin SDK tutorial — Target config", "plugin-development/tutorial/03-target", "Define a plugin's target configuration."},
	{"Plugin SDK tutorial — Create", "plugin-development/tutorial/05-create", "Implement the Create operation."},
	{"Plugin SDK reference — Schema annotations", "plugin-development/reference/schema", "Canonical PKL annotations reference."},
	{"Plugin SDK reference — Plugin interface", "plugin-development/reference/plugin-interface", "The ResourcePlugin contract."},
	{"Plugin SDK reference — Manifest", "plugin-development/reference/manifest", "Plugin manifest (formae-plugin.pkl) format."},
}

// mcpDocResources is the curated set of in-server MCP doc resources served
// under the formae://docs/* URI scheme. These are read directly via
// ReadResource — they are NOT docs.formae.io URLs.
var mcpDocResources = []struct {
	URI  string
	Desc string
}{
	{"formae://docs/concepts", "Core concepts: stacks, targets, resources, formas, modes, drift, discovery."},
	{"formae://docs/pkl-primer", "Minimal PKL syntax for reading and writing forma files."},
	{"formae://docs/forma-anatomy", "Forma file structure: the forma{}, stacks, targets, and resources blocks."},
	{"formae://docs/annotations", "Schema annotations: @formae.ResourceHint, @formae.FieldHint, @formae.Resolvable."},
	{"formae://docs/query-syntax", "Query syntax (Bluge field:value pairs) for list_resources and similar tools."},
	{"formae://docs/troubleshooting", "Common error messages, causes, and remediation steps."},
	{"formae://docs/examples", "Browsable plugin example index; use list_plugin_examples / get_plugin_example to fetch."},
	{"formae://docs/forma-structure", "Recommended file layout for forma projects (main.pkl, modules/, vars.pkl, etc.)."},
	{"formae://docs/stack-design", "Stack design guide: reconciliation boundaries, nested targets, and policy placement."},
	{"formae://docs/authoring-pitfalls", "Common authoring mistakes: wrong forma block, reconcile vs patch, label collisions."},
}

// docsIndexDoc renders the canonical documentation index served at
// formae://docs/index.
func docsIndexDoc() string {
	var b strings.Builder
	b.WriteString("# Formae Documentation Index\n\n")
	b.WriteString("Canonical documentation URLs. Use these exact links — do not construct, shorten, or guess documentation URLs.\n\n")

	b.WriteString("## External documentation (docs.formae.io)\n\n")
	for _, p := range docPages {
		fmt.Fprintf(&b, "- [%s](%s) — %s\n", p.Title, docURL(p.Path), p.Desc)
	}

	b.WriteString("\n## MCP doc resources (read directly)\n\n")
	b.WriteString("The following resources are served in-server and can be read via ReadResource without a network call.\n\n")
	for _, r := range mcpDocResources {
		fmt.Fprintf(&b, "- %s — %s\n", r.URI, r.Desc)
	}

	return b.String()
}
