package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

func blocks(t *testing.T, res *mcp.CallToolResult) []string {
	t.Helper()
	out := make([]string, 0, len(res.Content))
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", c)
		}
		out = append(out, tc.Text)
	}
	return out
}

// hostedServerFor wires a server whose resolved context is hosted but whose
// transport lands on srv, so a handler can be driven end to end.
func hostedServerFor(t *testing.T, srv *httptest.Server) (*Server, execctx.Context) {
	t.Helper()
	ec := execctx.Context{
		ProfileName: "acme-prod",
		Conn:        config.Hosted{Endpoint: srv.URL, Installation: testInstallation},
		Credential:  secret.New("Bearer live-token"),
		FormaeBin:   "/usr/bin/formae",
	}
	s := New("")
	s.ctxResolver = &stubResolver{ec: ec}
	// The hosted arm validates its endpoint against a compile-time origin, so a
	// handler test has to build its client through the seam rather than through
	// the guard. The guard itself is covered directly in routing_test.go.
	s.newClient = func(ec execctx.Context) (*FormaeClient, error) {
		c := newTestHostedClientAt(srv, "Bearer live-token", nil)
		return c, nil
	}
	return s, ec
}

func TestHostedResultCarriesAttributionWithoutTouchingThePayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"label":"default"}]`))
	}))
	defer srv.Close()

	s, _ := hostedServerFor(t, srv)
	res, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", blocks(t, res))
	}

	got := blocks(t, res)
	if len(got) != 2 {
		t.Fatalf("want the payload and one attribution block, got %d: %v", len(got), got)
	}
	if got[0] != `[{"label":"default"}]` {
		t.Errorf("the first block must still be exactly the agent's payload, got %q", got[0])
	}
	if !strings.Contains(got[1], testInstallation) || !strings.Contains(got[1], "acme-prod") {
		t.Errorf("attribution must name the installation and the profile: %q", got[1])
	}
	if !strings.Contains(got[1], "answered") {
		t.Errorf("a call the agent answered should say so: %q", got[1])
	}
}

func TestClassicResultCarriesNoAttribution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"label":"default"}]`))
	}))
	defer srv.Close()

	s := New("")
	s.ctxResolver = &stubResolver{ec: execctx.Context{
		ProfileName: "dev",
		Conn:        config.Classic{URL: srv.URL},
		FormaeBin:   "/usr/bin/formae",
	}}

	res, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := blocks(t, res); len(got) != 1 {
		t.Fatalf("a classic result must carry one block, got %d: %v", len(got), got)
	}
}

// An agent error is still an answer, and saying so is what tells an operator
// the installation is reachable and rejecting them rather than unreachable.
func TestAnAgentErrorStillCountsAsAnswered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, _ := hostedServerFor(t, srv)
	res, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("a 500 must be an error result")
	}

	got := blocks(t, res)
	if len(got) != 2 || !strings.Contains(got[1], "answered") {
		t.Fatalf("an answered failure must be attributed as answered: %v", got)
	}
}

// The case the attribution exists for. A mutation whose transport fails after
// the request went out does not establish that the agent did nothing.
func TestAMutationThatFailsAfterDispatchSaysItMayHaveActed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Answer nothing and drop the connection, so the failure lands after
		// the request has already been sent.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test server does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	s, _ := hostedServerFor(t, srv)
	res, _, err := s.handleForceSync(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("a dropped connection must be an error result")
	}

	got := blocks(t, res)
	if len(got) != 2 {
		t.Fatalf("want the error and one attribution block, got %v", got)
	}
	if !strings.Contains(got[1], "may already have taken effect") {
		t.Errorf("a post-dispatch failure must not claim the agent did nothing: %q", got[1])
	}
	if strings.Contains(got[1], "answered") {
		t.Errorf("nothing answered, so the attribution must not say so: %q", got[1])
	}
	if strings.Contains(got[1], "was sent to installation") {
		t.Errorf("the MCP cannot know the installation itself received it: %q", got[1])
	}
}

// Resolved is not addressed. An apply whose forma file fails to evaluate has a
// destination and contacted nothing; claiming otherwise would send an operator
// to check an installation for work that cannot exist.
func TestAFailureBeforeDispatchSaysNothingWasSent(t *testing.T) {
	s, _ := hostedServerFor(t, httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })))

	res, _, err := s.handleApplyForma(context.Background(), nil, tools.ApplyFormaInput{
		FilePath: "/nonexistent/forma.pkl",
		Mode:     "reconcile",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatal("a missing forma file must be an error result")
	}

	got := blocks(t, res)
	if len(got) != 2 {
		t.Fatalf("want the error and one attribution block, got %v", got)
	}
	if !strings.Contains(got[1], "nothing was sent") {
		t.Errorf("a pre-dispatch failure must say nothing was sent: %q", got[1])
	}
}

// Before a context resolves there is no destination to name.
func TestAFailureBeforeResolutionCarriesNoAttribution(t *testing.T) {
	s := New("")
	s.ctxResolver = &stubResolver{
		ec:  execctx.Context{FormaeBin: "/usr/bin/formae"},
		err: &config.AmbiguousProfileError{Candidates: []string{"a", "b"}, Active: "a"},
	}

	res, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := blocks(t, res); len(got) != 1 {
		t.Fatalf("an unresolved call has no destination to name, got %v", got)
	}
}

func TestNoResultCarriesTheCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ec := execctx.Context{
		ProfileName: "acme-prod",
		Conn:        config.Hosted{Endpoint: srv.URL, Installation: testInstallation},
		Credential:  secret.New("Bearer sup3rs3cr3t"),
		FormaeBin:   "/usr/bin/formae",
	}
	s := New("")
	s.ctxResolver = &stubResolver{ec: ec}
	s.newClient = func(execctx.Context) (*FormaeClient, error) {
		return newTestHostedClientAt(srv, "Bearer sup3rs3cr3t", nil), nil
	}

	res, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	for _, b := range blocks(t, res) {
		if strings.Contains(b, "sup3rs3cr3t") {
			t.Fatalf("a result leaked the credential: %s", b)
		}
	}
}

// A structural check, in the spirit of TestNoDirectHTTPConstruction. Every
// result an agent-backed handler returns must be attributed, and a per-handler
// audit is precisely how one of them ends up not being.
//
// It checks each return rather than merely whether the handler mentions
// attribute() anywhere. The weaker form passes on a handler whose client-build
// failure is attributed and whose success path is not, which is the mistake
// most likely to be made.
func TestEveryAgentBackedHandlerAttributes(t *testing.T) {
	handler := regexp.MustCompile(`(?s)func \(s \*Server\) (handle\w+)\([^)]*\) \((?:res )?\*mcp\.CallToolResult[^{]*\{(.*?)\n\}`)
	returnsResult := regexp.MustCompile(`return (?:attribute\([^,]+, )?(jsonResult|textResult|errorResult|withNotice)\(`)

	for _, file := range []string{
		"server.go", "policy.go", "policy_standalone_tools.go", "profile_tools.go",
	} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for _, m := range handler.FindAllStringSubmatch(string(src), -1) {
			name, body := m[1], m[2]
			reachesAgent := strings.Contains(body, "s.clientFor(") ||
				strings.Contains(body, "s.newClient(") ||
				strings.Contains(body, "s.fetchPolicies(") ||
				strings.Contains(body, `"extract"`)
			if !reachesAgent {
				continue
			}
			// A handler may attribute every path at once with a deferred
			// assignment to a named result, which the per-return check cannot
			// see and does not need to.
			if strings.Contains(body, "defer func() { res = attribute(") {
				continue
			}
			// Only returns after the client exists are checked. Before that
			// there is genuinely nothing to name — an input-validation failure
			// or a resolution failure has no destination — and the one case in
			// between, a forma file that fails to evaluate after resolution, is
			// covered behaviourally rather than structurally.
			var haveClient bool
			for _, line := range strings.Split(body, "\n") {
				if strings.Contains(line, "s.newClient(") || strings.Contains(line, "s.fetchPolicies(") {
					haveClient = true
					continue
				}
				if !haveClient || !returnsResult.MatchString(line) {
					continue
				}
				if !strings.Contains(line, "attribute(") {
					t.Errorf("%s in %s returns an unattributed result once a client exists; "+
						"a hosted caller would not learn which installation acted:\n\t%s",
						name, file, strings.TrimSpace(line))
				}
			}
		}
	}
}

// Extract reaches the agent through the CLI, so its output never passes
// through the executor's scrub. Its diagnostics are the whole value of the
// failure and are kept, bounded, with the credential we handed the process
// removed.
func TestExtractFailureOutputIsBoundedAndScrubbed(t *testing.T) {
	t.Run("the credential we passed is removed", func(t *testing.T) {
		got := safeSubprocessOutput(
			[]byte("plugin refused: Authorization: Bearer sup3rs3cr3t"),
			secret.New("Bearer sup3rs3cr3t"))

		if strings.Contains(got, "sup3rs3cr3t") {
			t.Fatalf("extract output leaked the credential: %s", got)
		}
		if !strings.Contains(got, "plugin refused") {
			t.Errorf("the diagnostics are the point and must survive: %s", got)
		}
	})

	t.Run("output is bounded", func(t *testing.T) {
		got := safeSubprocessOutput(bytes.Repeat([]byte("x"), maxSubprocessOutput*4), secret.Value{})

		if len(got) > maxSubprocessOutput+len("\n… truncated") {
			t.Fatalf("output was not bounded: %d bytes", len(got))
		}
		if !strings.Contains(got, "truncated") {
			t.Errorf("a truncated result should say so")
		}
	})

	t.Run("a classic call has no credential to remove", func(t *testing.T) {
		if got := safeSubprocessOutput([]byte("plain diagnostics"), secret.Value{}); got != "plain diagnostics" {
			t.Fatalf("got %q", got)
		}
	})
}
