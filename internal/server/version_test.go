package server

import (
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/version"
)

// The MCP handshake Implementation.Version must come from the single
// internal/version source of truth, not a hardcoded constant. Assert the
// wiring at the construction seam rather than reaching into SDK internals.
func TestImplementation_VersionSourcedFromVersionPackage(t *testing.T) {
	impl := implementation()

	if impl.Name != serverName {
		t.Errorf("implementation().Name = %q, want %q", impl.Name, serverName)
	}
	if impl.Version != version.String() {
		t.Errorf("implementation().Version = %q, want %q", impl.Version, version.String())
	}
}
