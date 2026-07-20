// Package version exposes the formae-mcp binary version through a single,
// build-injectable source of truth shared by the CLI --version flag and the
// MCP handshake Implementation.Version.
package version

import "runtime/debug"

// version is the build version, stamped at link time via:
//
//	-ldflags "-X github.com/platform-engineering-labs/formae-mcp/internal/version.version=<v>"
//
// It is unexported so no other package can mutate process-wide version state;
// callers read it through String().
var version = ""

// String returns the binary version, resolved in order: the ldflag-stamped
// value if present, otherwise the Go module version from build info (so
// `go install …@latest` reports the module tag with no ldflags), otherwise "dev".
func String() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
