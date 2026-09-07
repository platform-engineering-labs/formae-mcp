package execctx

import (
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
)

func TestResolveClassic(t *testing.T) {
	r := &Resolver{
		endpoint: func(p string) (string, string, error) { return "http://localhost", "49684", nil },
		bin:      formaebin.BinResolver{BundledPath: "/bundle/formae", LookPath: func(string) (string, error) { return "/usr/bin/formae", nil }, Exists: func(string) bool { return false }},
	}
	ctx, err := r.Resolve("dev")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ctx.Mode != formaebin.Classic {
		t.Errorf("Mode = %v, want Classic", ctx.Mode)
	}
	if ctx.FormaeBin != "/usr/bin/formae" {
		t.Errorf("FormaeBin = %q, want /usr/bin/formae", ctx.FormaeBin)
	}
	if ctx.URL != "http://localhost" || ctx.Port != "49684" {
		t.Errorf("endpoint = %s:%s", ctx.URL, ctx.Port)
	}
	if ctx.ProfileName != "dev" {
		t.Errorf("ProfileName = %q", ctx.ProfileName)
	}
}
