package execctx

import (
	"context"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
)

func testBin() formaebin.BinResolver {
	return formaebin.BinResolver{
		Getenv:   func(string) string { return "" },
		LookPath: func(string) (string, error) { return "/usr/bin/formae", nil },
	}
}

// TestResolveCarriesTheConnection pins that the context carries what the CLI
// resolved, including the effective profile name the CLI reported rather than
// the one that was asked for.
func TestResolveCarriesTheConnection(t *testing.T) {
	r := &Resolver{
		resolve: func(_ context.Context, bin, profileName string) (config.Resolved, error) {
			if bin != "/usr/bin/formae" {
				t.Errorf("resolve called with bin %q, want the resolved binary", bin)
			}
			if profileName != "dev" {
				t.Errorf("resolve called with profile %q, want %q", profileName, "dev")
			}
			return config.Resolved{
				Profile: "dev-effective",
				Conn:    config.Classic{URL: "http://localhost", Port: 49684},
			}, nil
		},
		bin: testBin(),
	}

	ec, err := r.Resolve(context.Background(), "dev")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ec.ProfileName != "dev-effective" {
		t.Errorf("ProfileName = %q, want the name the CLI reported", ec.ProfileName)
	}
	if ec.FormaeBin != "/usr/bin/formae" {
		t.Errorf("FormaeBin = %q", ec.FormaeBin)
	}
	if ec.Conn != config.Connection(config.Classic{URL: "http://localhost", Port: 49684}) {
		t.Errorf("Conn = %#v", ec.Conn)
	}
}

func TestResolveHosted(t *testing.T) {
	hosted := config.Hosted{
		Endpoint:     config.HostedOrigin,
		Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa",
	}
	r := &Resolver{
		resolve: func(context.Context, string, string) (config.Resolved, error) {
			return config.Resolved{Profile: "prod", Conn: hosted}, nil
		},
		bin: testBin(),
	}

	ec, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ec.Conn != config.Connection(hosted) {
		t.Errorf("Conn = %#v, want the hosted arm", ec.Conn)
	}
}
