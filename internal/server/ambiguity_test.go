package server

import (
	"context"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// The CLI decides ambiguity, because it is the only side that can settle it
// before a credential is minted. The MCP's job is to turn that refusal into
// something the caller can act on rather than an opaque failure.
//
// Elicitation and a remembered per-session profile are deliberately not here.
// This message works in every MCP client, needs no session state and no
// capability negotiation, and is what an elicitation path would fall back to
// anyway.
func TestAmbiguityBecomesAnActionableInstruction(t *testing.T) {
	s := New("")
	s.ctxResolver = &stubResolver{
		ec: execctx.Context{FormaeBin: "/usr/bin/formae"},
		err: &config.AmbiguousProfileError{
			Candidates: []string{"acme-prod", "acme-staging"},
			Active:     "acme-prod",
		},
	}

	res, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler returned a transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("ambiguity must reach the caller as a failed call it can retry")
	}

	text := textContent(t, res)
	for _, want := range []string{"acme-prod", "acme-staging", "active", "profile"} {
		if !strings.Contains(text, want) {
			t.Errorf("the instruction must mention %q so the caller can retry: %s", want, text)
		}
	}
}

// An explicit profile settles the choice, so the caller's retry has to reach
// the resolver as the named profile rather than being dropped.
func TestAnExplicitProfileIsPassedThroughOnTheRetry(t *testing.T) {
	r := &stubResolver{ec: execctx.Context{
		ProfileName: "acme-staging",
		Conn:        config.Classic{URL: "http://localhost", Port: 49684},
		FormaeBin:   "/usr/bin/formae",
	}}
	s := New("")
	s.ctxResolver = r

	if _, err := s.clientFor(context.Background(), "acme-staging"); err != nil {
		t.Fatalf("clientFor: %v", err)
	}
	if r.sawProfile != "acme-staging" {
		t.Fatalf("the named profile did not reach the resolver, got %q", r.sawProfile)
	}
}
