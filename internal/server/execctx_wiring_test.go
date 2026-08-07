package server

import (
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
)

// evalFormaFile must run the binary from the resolved context, not a bare
// "formae" — the mechanism that later lets hosted and classic use different
// binaries without the eval/submit split disagreeing.
func TestEvalFormaFileUsesContextBinary(t *testing.T) {
	ctx := execctx.Context{Mode: formaebin.Classic, FormaeBin: "/nonexistent/marker-formae"}
	_, err := evalFormaFile(ctx, "does-not-matter.pkl")
	if err == nil {
		t.Fatal("expected error from missing binary")
	}
	if !strings.Contains(err.Error(), "marker-formae") {
		t.Fatalf("error did not reference the context binary: %v", err)
	}
}
