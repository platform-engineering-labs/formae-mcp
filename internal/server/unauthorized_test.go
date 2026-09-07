package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

// recordingRefresher stands in for the resolver seam, counting what the 401
// path asked for. Counting the calls is the point: an implementation that
// reaches past the seam, or that refreshes when it should not, is invisible to
// an assertion that only looks at the error.
type recordingRefresher struct {
	calls int
	next  execctx.Context
	err   error
}

func (r *recordingRefresher) refresh(context.Context) (execctx.Context, error) {
	r.calls++
	return r.next, r.err
}

// hostedAt is what a refresh resolves to: the same installation at the same
// endpoint the route already holds. The tests aim the transport at httptest, so
// the refreshed context has to name that endpoint too — otherwise every refresh
// would look like the target moving, and the binding check would pass these
// tests for the wrong reason.
func hostedAt(endpoint, credential string) execctx.Context {
	return execctx.Context{
		ProfileName: "prod",
		Conn: config.Hosted{
			Endpoint:     endpoint,
			Installation: testInstallation,
		},
		Credential: secret.New(credential),
		FormaeBin:  "/usr/bin/formae",
	}
}

// unauthorizedThenOK answers 401 until the request carries wantAuth, then 200.
// It records every Authorization it saw, so a retry can be checked to have used
// the refreshed credential rather than merely to have happened.
type unauthorizedThenOK struct {
	wantAuth string
	seen     []string
}

func (h *unauthorizedThenOK) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	h.seen = append(h.seen, auth)
	if auth != h.wantAuth {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func TestRetryableRequestRefreshesAndRetriesWithTheNewCredential(t *testing.T) {
	handler := &unauthorizedThenOK{wantAuth: "Bearer refreshed"}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	rr := &recordingRefresher{next: hostedAt(srv.URL, "Bearer refreshed")}
	c := newTestHostedClient(t, srv, "Bearer stale", rr.refresh)

	_, status, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want the retry to succeed", status)
	}
	if rr.calls != 1 {
		t.Errorf("resolver calls = %d, want exactly one refresh", rr.calls)
	}
	if len(handler.seen) != 2 {
		t.Fatalf("requests = %d, want the original and one retry", len(handler.seen))
	}
	if handler.seen[0] != "Bearer stale" || handler.seen[1] != "Bearer refreshed" {
		t.Errorf("the retry did not carry the refreshed credential: %v", handler.seen)
	}
}

// A second 401 is reported as unauthorized without invention. It has no plugin
// error behind it and can mean expiry, wrong audience, wrong issuer or a
// malformed credential, so nothing here can say which.
func TestASecondUnauthorizedIsNotRefreshedAgain(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	rr := &recordingRefresher{next: hostedAt(srv.URL, "Bearer also-rejected")}
	c := newTestHostedClient(t, srv, "Bearer stale", rr.refresh)

	_, status, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want the unauthorized to be reported", status)
	}
	if rr.calls != 1 {
		t.Errorf("resolver calls = %d, want exactly one refresh", rr.calls)
	}
	if requests != 2 {
		t.Errorf("requests = %d, want exactly one retry", requests)
	}
}

// A mutation refreshes so the next call succeeds, and returns the original
// failure. Nothing establishes that a 401 always precedes dispatch, and
// duplicating an infrastructure mutation to save one error is the wrong trade.
func TestAMutationRefreshesButNeverRetries(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	rr := &recordingRefresher{next: hostedAt(srv.URL, "Bearer refreshed")}
	c := newTestHostedClient(t, srv, "Bearer stale", rr.refresh)

	_, status, err := c.do(context.Background(), request{Method: "POST", Path: "/api/v1/commands"}, noRetry)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want the original failure", status)
	}
	if rr.calls != 1 {
		t.Errorf("resolver calls = %d, want the refresh to still happen", rr.calls)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want no second attempt", requests)
	}
}

// A denied installation or tenant is an authorization failure, not an expired
// session. Asserting the resolver count rather than the error text is what
// makes this a real check: an implementation that refreshed and then reported
// the 403 anyway would pass a text assertion.
func TestForbiddenNeverReauthenticates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	rr := &recordingRefresher{next: hostedAt(srv.URL, "Bearer refreshed")}
	c := newTestHostedClient(t, srv, "Bearer live", rr.refresh)

	_, status, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if status != http.StatusForbidden {
		t.Errorf("status = %d", status)
	}
	if rr.calls != 0 {
		t.Fatalf("a 403 triggered %d resolver calls; it must trigger none", rr.calls)
	}
}

// A refresh that comes back pointing somewhere else is abandoned rather than
// retried. Without this the refresh path silently re-introduces the skew that
// resolving configuration and credentials together exists to prevent.
func TestARefreshThatMovesTheTargetAborts(t *testing.T) {
	// Each case is built against the endpoint the route actually holds, so it
	// fails the check it is named for. Hard-coding the real origin here would
	// make every case fail the endpoint comparison first, and the credential
	// case in particular would pass without ever exercising its own check.
	moved := map[string]func(endpoint string) execctx.Context{
		"a different installation": func(endpoint string) execctx.Context {
			return execctx.Context{
				ProfileName: "prod",
				Conn: config.Hosted{
					Endpoint:     endpoint,
					Installation: "2ZaBcDeFgHiJkLmNoPqRsTuVwXy",
				},
				Credential: secret.New("Bearer refreshed"),
			}
		},
		"a different origin": func(string) execctx.Context {
			return execctx.Context{
				ProfileName: "prod",
				Conn: config.Hosted{
					Endpoint:     "https://other.formae.ai",
					Installation: testInstallation,
				},
				Credential: secret.New("Bearer refreshed"),
			}
		},
		"a mode that flipped to classic": func(string) execctx.Context {
			return execctx.Context{
				ProfileName: "prod",
				Conn:        config.Classic{URL: "http://localhost", Port: 49684},
			}
		},
		"a refresh that returned no credential": func(endpoint string) execctx.Context {
			return execctx.Context{
				ProfileName: "prod",
				Conn: config.Hosted{
					Endpoint:     endpoint,
					Installation: testInstallation,
				},
			}
		},
	}
	for name, build := range moved {
		t.Run(name, func(t *testing.T) {
			var requests int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer srv.Close()

			rr := &recordingRefresher{next: build(srv.URL)}
			c := newTestHostedClient(t, srv, "Bearer stale", rr.refresh)

			_, _, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)

			if err == nil {
				t.Fatal("a refresh that moved the target must abort")
			}
			if requests != 1 {
				t.Errorf("requests = %d, want no attempt against the moved target", requests)
			}
		})
	}
}

// The connection-moved abort names what happened, because "unauthorized" would
// send the reader looking at credentials rather than at their profile.
func TestTheConnectionMovedErrorSaysSo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	rr := &recordingRefresher{next: execctx.Context{
		ProfileName: "prod",
		Conn: config.Hosted{
			Endpoint:     config.HostedOrigin,
			Installation: "2ZaBcDeFgHiJkLmNoPqRsTuVwXy",
		},
		Credential: secret.New("Bearer refreshed"),
	}}
	c := newTestHostedClient(t, srv, "Bearer stale", rr.refresh)

	_, _, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)

	if !errors.Is(err, errConnectionMoved) {
		t.Fatalf("want errConnectionMoved, got %v", err)
	}
}

// A refresh that fails outright surfaces its own error rather than the 401.
func TestARefreshFailureSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	rr := &recordingRefresher{err: errors.New("the auth plugin is not installed")}
	c := newTestHostedClient(t, srv, "Bearer stale", rr.refresh)

	_, _, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)

	if err == nil || !strings.Contains(err.Error(), "auth plugin") {
		t.Fatalf("want the refresh failure, got %v", err)
	}
}

// The MCP sends a self-hosted agent no credential, so a 401 from one is
// somebody else's problem and there is nothing a retry would change.
func TestAClassicUnauthorizedIsReturnedAsIs(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	_, status, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, retryOnce)

	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d", status)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want no retry", requests)
	}
}

// A retryable request carrying a body would replay an already-consumed reader
// and silently send an empty one, which looks like a successful request rather
// than a corrupted one. Refused outright instead.
func TestARetryableRequestMayNotCarryABody(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	_, _, err := c.do(context.Background(), request{
		Method: "POST",
		Path:   "/api/v1/commands",
		Body:   strings.NewReader("payload"),
	}, retryOnce)

	if !errors.Is(err, errRetryableBody) {
		t.Fatalf("want errRetryableBody, got %v", err)
	}
	if requests != 0 {
		t.Errorf("the request must be refused before it is sent, got %d requests", requests)
	}
}
