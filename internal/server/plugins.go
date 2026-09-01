package server

import "regexp"

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
