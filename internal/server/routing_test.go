package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

const testInstallation = "3HzFPXfPDGhwLJJVtaHbmFs6vLa"

// hostedCtx is a resolved hosted context pointing at srv rather than the real
// edge, so the routing behaviour can be exercised against httptest.
func hostedCtx(credential string) execctx.Context {
	return execctx.Context{
		ProfileName: "prod",
		Conn: config.Hosted{
			Endpoint:     config.HostedOrigin,
			Installation: testInstallation,
		},
		Credential: secret.New(credential),
		FormaeBin:  "/usr/bin/formae",
	}
}

// newTestHostedClient builds a hosted client whose requests land on srv. The
// endpoint the routing validated is the real origin; only the transport is
// redirected, so the header and credential behaviour under test is the same
// behaviour that would reach the edge.
func newTestHostedClient(t *testing.T, srv *httptest.Server, credential string, refresh refresher) *FormaeClient {
	t.Helper()
	return newTestHostedClientAt(srv, credential, refresh)
}

// newTestHostedClientAt builds the same client without a *testing.T, for the
// places that need one inside a seam closure.
func newTestHostedClientAt(srv *httptest.Server, credential string, refresh refresher) *FormaeClient {
	return &FormaeClient{
		route: &hostedRoute{
			endpoint:     srv.URL,
			installation: testInstallation,
			credential:   secret.New(credential),
			refreshFn:    refresh,
		},
		httpClient: &http.Client{
			Transport:     srv.Client().Transport,
			CheckRedirect: refuseRedirects,
		},
	}
}

func TestHostedRequestsCarryTheInstallationExactlyOnce(t *testing.T) {
	var values []string
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values = r.Header.Values("Formae-Installation")
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestHostedClient(t, srv, "Bearer live-token", nil)
	// The request names both headers itself. This is the case that separates
	// Set from Add: on a fresh header the two are indistinguishable, and a
	// second routing value is not a warning at the edge but a rejection that
	// happens after a router has already been chosen, so it surfaces somewhere
	// else entirely. It also pins that the routing decorates last and wins,
	// rather than a caller being able to redirect a credentialled request.
	if _, _, err := c.do(context.Background(), request{
		Method: "GET",
		Path:   "/api/v1/health",
		Headers: map[string]string{
			"Formae-Installation": "2ZaBcDeFgHiJkLmNoPqRsTuVwXy",
			"Authorization":       "Bearer somebody-elses",
		},
	}, noRetry); err != nil {
		t.Fatalf("do: %v", err)
	}

	if len(values) != 1 {
		t.Fatalf("Formae-Installation must be set exactly once, got %d values: %v", len(values), values)
	}
	if values[0] != testInstallation {
		t.Errorf("Formae-Installation = %q, want the routing's own", values[0])
	}
	if auth != "Bearer live-token" {
		t.Errorf("Authorization = %q, want the routing's own", auth)
	}
}

func TestClassicRequestsCarryNeitherHeader(t *testing.T) {
	var installation, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		installation = r.Header.Get("Formae-Installation")
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	if _, _, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, noRetry); err != nil {
		t.Fatalf("do: %v", err)
	}

	if installation != "" {
		t.Errorf("a classic request must carry no routing header, got %q", installation)
	}
	if auth != "" {
		t.Errorf("a classic request must carry no credential, got %q", auth)
	}
}

// Go strips Authorization across a cross-host redirect but forwards custom
// headers, so the routing header would follow one. Every 3xx is refused, and
// the target must never be contacted.
func TestHostedRefusesEveryRedirect(t *testing.T) {
	for _, code := range []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var followed bool
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				followed = true
				w.WriteHeader(http.StatusOK)
			}))
			defer target.Close()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/api/v1/health", code)
			}))
			defer srv.Close()

			c := newTestHostedClient(t, srv, "Bearer live-token", nil)
			_, _, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, noRetry)

			if err == nil {
				t.Fatal("a redirect on a hosted request must be an error")
			}
			if !strings.Contains(err.Error(), "routing") {
				t.Errorf("the error should name routing as the cause: %v", err)
			}
			if followed {
				t.Fatal("the redirect target was contacted")
			}
		})
	}
}

// A classic connection keeps Go's default redirect handling: nothing about its
// behaviour changes in this slice.
func TestClassicStillFollowsRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v1/health", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	body, status, err := c.do(context.Background(), request{Method: "GET", Path: "/api/v1/health"}, noRetry)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if status != http.StatusOK || !strings.Contains(string(body), "ok") {
		t.Fatalf("classic redirect handling changed: status %d body %s", status, body)
	}
}

// The MCP re-validates what the CLI resolved, because it exposes tools that
// write profiles: a model fed hostile input can author one.
func TestClientConstructionRevalidatesTheHostedConnection(t *testing.T) {
	cases := map[string]config.Hosted{
		"a foreign endpoint":       {Endpoint: "https://evil.example", Installation: testInstallation},
		"a malformed installation": {Endpoint: config.HostedOrigin, Installation: "not-an-installation"},
		"the retired uuid form":    {Endpoint: config.HostedOrigin, Installation: "3f2b8c14-0000-4000-8000-000000000000"},
		"an http endpoint":         {Endpoint: "http://cloud.formae.ai", Installation: testInstallation},
		"an endpoint with a path":  {Endpoint: "https://cloud.formae.ai/api", Installation: testInstallation},
	}
	for name, conn := range cases {
		t.Run(name, func(t *testing.T) {
			ec := execctx.Context{ProfileName: "prod", Conn: conn, Credential: secret.New("Bearer x")}

			if _, err := newClientFromCtx(ec, nil); err == nil {
				t.Fatalf("%s must be refused before it can reach a header", name)
			}
		})
	}
}

// A hosted connection with no credential cannot authenticate, so it is not a
// usable connection. The decoder refuses the shape; this refuses it again at
// the point where a request would otherwise go out unauthenticated.
func TestHostedClientRequiresACredential(t *testing.T) {
	ec := hostedCtx("")

	if _, err := newClientFromCtx(ec, nil); err == nil {
		t.Fatal("a hosted connection with no credential must be refused")
	}
}

// A credential must not reach a rendering of the client or its routing.
func TestClientDoesNotRenderTheCredential(t *testing.T) {
	c, err := newClientFromCtx(hostedCtx("Bearer sup3rs3cr3t"), nil)
	if err != nil {
		t.Fatalf("newClientFromCtx: %v", err)
	}

	for _, rendered := range []string{
		fmt.Sprintf("%v", c.route),
		fmt.Sprintf("%+v", c.route),
		fmt.Sprintf("%#v", c.route),
	} {
		if strings.Contains(rendered, "sup3rs3cr3t") {
			t.Fatalf("a rendering of the routing leaked the credential: %s", rendered)
		}
	}

	out, err := json.Marshal(c.route)
	if err != nil {
		t.Fatalf("marshalling the routing: %v", err)
	}
	if strings.Contains(string(out), "sup3rs3cr3t") {
		t.Fatalf("a JSON rendering of the routing leaked the credential: %s", out)
	}
}
