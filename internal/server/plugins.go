package server

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// schemaVersionPattern matches the two installed-version forms whose schema
// package is published at the canonical X.Y.Z coordinate: a plain release, and
// a -dev build, which publishes over that same coordinate.
//
// Every other prerelease is deliberately excluded rather than truncated. A
// -rc.1 or -beta.2 build has no such convention, so truncating it would name a
// different published version, and a model handed that would pin a schema the
// agent is not running.
var schemaVersionPattern = regexp.MustCompile(`^(\d+\.\d+\.\d+)(-dev(\.\d+)?)?$`)

// canonicalSchemaVersion returns the version a project should pin for a plugin
// the agent reports at this installed version, and whether one can be named at
// all. A false means say so, never guess.
func canonicalSchemaVersion(installed string) (string, bool) {
	m := schemaVersionPattern.FindStringSubmatch(installed)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// renderAgentPlugins says what the installation reports and what the caller may
// do with it.
//
// Text rather than JSON, for the reason renderConnections is text: the point is
// the capability the listing implies, and a payload leaves the model to infer
// that for itself. The mode changes the meaning of the same list, so it is
// stated first and never left to be guessed from the contents.
func renderAgentPlugins(hosted bool, plugins []agentPlugin) string {
	resources, auth, bundles, unknown := classifyPlugins(plugins)

	var b strings.Builder
	b.WriteString(pluginsHeader(hosted, len(plugins), len(resources)))

	if len(resources) > 0 {
		b.WriteString("\nAuthorable resource plugins:\n")
		for _, p := range resources {
			if pin, ok := canonicalSchemaVersion(p.InstalledVersion); ok {
				fmt.Fprintf(&b, "  %s (installed %s) - pin the schema package at %s@%s\n",
					p.Name, p.InstalledVersion, p.Name, pin)
				continue
			}
			fmt.Fprintf(&b, "  %s (installed %s) - no schema version can be named for this "+
				"build; do not write a dependency for it, ask the user\n",
				p.Name, p.InstalledVersion)
		}
	}
	if len(auth) > 0 {
		b.WriteString("\nAuth plugins, which are not authored against: ")
		b.WriteString(joinNameVersions(auth))
		b.WriteString("\n")
	}
	if len(bundles) > 0 {
		b.WriteString("Installed bundles, which are not plugins: ")
		b.WriteString(joinNameVersions(bundles))
		b.WriteString("\n")
	}
	if len(unknown) > 0 {
		b.WriteString("Reported with a kind or type this server does not recognise, so treated " +
			"as not authorable: ")
		b.WriteString(joinNameVersions(unknown))
		b.WriteString("\n")
	}
	if len(resources) > 0 {
		b.WriteString("\nThis is what the installation reports. A schema coordinate here is the " +
			"one to resolve, not one already verified to exist: if resolving it fails, say so " +
			"rather than working around it.\n")
	}

	return b.String()
}

// classifyPlugins sorts the listing into what may be authored against and what
// may not.
//
// A resource plugin has to say so: anything whose kind or type this server does
// not recognise is reported as unrecognised rather than falling through to
// authorable. The listing comes from a remote agent and the decoder does not
// validate it, so a default-to-authorable branch would turn a field this server
// has never seen into a schema pin the user acts on. Kind is absent on plain
// plugins from some agents (it is omitempty on the wire), so an empty kind with
// a resource type is a plugin.
//
// Each group is sorted by name: the agent's own order comes from a registry
// merged with a filesystem scan, so it is not stable across installations or
// even across calls, and unstable output makes both model context and tests
// wobble for no reason.
func classifyPlugins(plugins []agentPlugin) (resources, auth, bundles, unknown []agentPlugin) {
	for _, p := range plugins {
		switch {
		case p.Kind == "metapackage":
			bundles = append(bundles, p)
		case p.Type == "auth":
			auth = append(auth, p)
		case p.Type == "resource" && (p.Kind == "" || p.Kind == "plugin"):
			resources = append(resources, p)
		default:
			unknown = append(unknown, p)
		}
	}
	for _, group := range [][]agentPlugin{resources, auth, bundles, unknown} {
		sort.Slice(group, func(i, j int) bool { return group[i].Name < group[j].Name })
	}
	return resources, auth, bundles, unknown
}

// pluginsHeader states the mode and what this particular listing means under
// it, before any of the listing is shown.
//
// "Nothing was reported" and "things were reported but none of them can be
// authored against" are different facts and get different sentences. Collapsing
// them would either hide the auth plugins and bundles that were reported, or
// claim an installation answered with nothing when it did not.
func pluginsHeader(hosted bool, total, authorable int) string {
	if hosted {
		switch {
		case total == 0:
			return "The set of plugins available on this installation could not be established: " +
				"the installation reported nothing at all, and a hosted installation with no " +
				"plugins is not a real state. Do NOT read this as an empty set and do NOT fall " +
				"back to the hub catalogue, which lists plugins this installation cannot " +
				"install. Say the set could not be established and stop there.\n"
		case authorable == 0:
			return "This installation reported plugins, but none that can be authored against " +
				"(they are listed below). A hosted installation with no resource plugins is " +
				"not a real state, so treat the authorable set as undetermined rather than " +
				"empty: say so and stop, and do not fall back to the hub catalogue.\n"
		}
		return "This is hosted formae. These are the plugins the installation reports as " +
			"installed, and they cannot be installed on demand, so this set is what authoring " +
			"can use. Do not offer to author a new plugin as a way to add one. If an intent " +
			"needs something absent, say it is not reported as installed on this installation, " +
			"say what the available set can do instead, and stop.\n"
	}
	switch {
	case total == 0:
		return "This agent reports no plugins installed. That is a real state for a self-hosted " +
			"agent, not a failure. Authoring and simulate need only schemas, so work can " +
			"continue; search_hub_plugins is the catalogue of what can be installed.\n"
	case authorable == 0:
		return "This agent reports no resource plugins installed, only what is listed below. " +
			"That is a real state for a self-hosted agent, not a failure. Authoring and " +
			"simulate need only schemas, so work can continue; search_hub_plugins is the " +
			"catalogue of what can be installed.\n"
	}
	return "This is a self-hosted agent. These are the plugins it has installed today; more can " +
		"be installed, and search_hub_plugins is the catalogue.\n"
}

func joinNameVersions(ps []agentPlugin) string {
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, p.Name+" "+p.InstalledVersion)
	}
	return strings.Join(parts, ", ")
}

// renderClassicListingUnavailable is the classic answer to a listing that could
// not be read. It is not an error: the mode is already known, the hub is the
// catalogue, and authoring needs only schemas, so the conversation continues
// with one fact fewer.
func renderClassicListingUnavailable(err error) string {
	return fmt.Sprintf("This is a self-hosted agent, and its installed plugin set could not be "+
		"read (%v). Nothing depends on it: authoring and simulate need only schema packages, "+
		"and search_hub_plugins is the catalogue. Continue, and if an apply later fails for a "+
		"missing plugin, the apply error names it.\n", err)
}
