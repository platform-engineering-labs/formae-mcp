package execctx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
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
		resolve: func(_ context.Context, bin, profileName string, forceRefresh bool) (config.Resolved, error) {
			if bin != "/usr/bin/formae" {
				t.Errorf("resolve called with bin %q, want the resolved binary", bin)
			}
			if profileName != "dev" {
				t.Errorf("resolve called with profile %q, want %q", profileName, "dev")
			}
			if forceRefresh {
				t.Error("an ordinary resolution must not force a refresh")
			}
			return config.Resolved{
				Profile: "dev-effective",
				Conn:    config.Classic{URL: "http://localhost", Port: 49684},
			}, nil
		},
		bin: testBin(),
	}

	ec, err := r.Resolve(context.Background(), "dev", false)
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
	if !ec.Credential.IsZero() {
		t.Error("a classic context must carry no credential")
	}
}

func TestResolveHosted(t *testing.T) {
	hosted := config.Hosted{
		Endpoint:     config.HostedOrigin,
		Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa",
	}
	r := &Resolver{
		resolve: func(context.Context, string, string, bool) (config.Resolved, error) {
			return config.Resolved{
				Profile:    "prod",
				Conn:       hosted,
				Credential: secret.New("Bearer live-token"),
			}, nil
		},
		bin: testBin(),
	}

	ec, err := r.Resolve(context.Background(), "", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ec.Conn != config.Connection(hosted) {
		t.Errorf("Conn = %#v, want the hosted arm", ec.Conn)
	}
	if ec.Credential.Reveal() != "Bearer live-token" {
		t.Errorf("Credential = %q", ec.Credential.Reveal())
	}
}

// The 401 path re-resolves with the credential refreshed, so the flag has to
// reach the CLI rather than being dropped at this seam.
func TestResolveForwardsAForcedRefresh(t *testing.T) {
	var saw bool
	r := &Resolver{
		resolve: func(_ context.Context, _, _ string, forceRefresh bool) (config.Resolved, error) {
			saw = forceRefresh
			return config.Resolved{
				Profile: "dev",
				Conn:    config.Classic{URL: "http://localhost", Port: 49684},
			}, nil
		},
		bin: testBin(),
	}

	if _, err := r.Resolve(context.Background(), "dev", true); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !saw {
		t.Fatal("a forced refresh did not reach the resolver")
	}
}

// The context gets logged and serialised in the ordinary course of debugging.
func TestContextDoesNotRenderTheCredential(t *testing.T) {
	ec := Context{
		ProfileName: "prod",
		Conn:        config.Hosted{Endpoint: config.HostedOrigin, Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa"},
		Credential:  secret.New("Bearer sup3rs3cr3t"),
		FormaeBin:   "/usr/bin/formae",
	}

	out, err := json.Marshal(ec)
	if err != nil {
		t.Fatalf("marshalling the context: %v", err)
	}
	if strings.Contains(string(out), "sup3rs3cr3t") {
		t.Fatalf("a JSON rendering of the context leaked the credential: %s", out)
	}
}
