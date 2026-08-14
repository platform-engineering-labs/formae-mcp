package server

import (
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

// evalFormaFile must run the binary from the resolved context, not a bare
// "formae", so one call makes one binary decision and eval and submit cannot
// disagree about which formae they used.
func TestEvalFormaFileUsesContextBinary(t *testing.T) {
	ec := execctx.Context{FormaeBin: "/nonexistent/marker-formae"}
	_, err := evalFormaFile(ec, "does-not-matter.pkl")
	if err == nil {
		t.Fatal("expected error from missing binary")
	}
	if !strings.Contains(err.Error(), "marker-formae") {
		t.Fatalf("error did not reference the context binary: %v", err)
	}
}
