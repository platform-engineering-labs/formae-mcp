package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The seven endpoints that translate a 404 into an empty payload. Under hosted
// the shared edge answers 404 for an unknown or unrouted installation, so
// reporting "nothing here" would present a routing failure as a plausible
// answer — the one failure mode nobody investigates.
var collectionCalls = map[string]struct {
	call      func(context.Context, *FormaeClient) (json.RawMessage, error)
	wantEmpty string
}{
	"ListResources": {
		call:      func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) { return c.ListResources(ctx, "") },
		wantEmpty: "[]",
	},
	"ListStacks": {
		call:      func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) { return c.ListStacks(ctx) },
		wantEmpty: "[]",
	},
	"ListPolicies": {
		call:      func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) { return c.ListPolicies(ctx) },
		wantEmpty: "[]",
	},
	"ListGenerators": {
		call:      func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) { return c.ListGenerators(ctx) },
		wantEmpty: "[]",
	},
	"ListTargets": {
		call:      func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) { return c.ListTargets(ctx, "") },
		wantEmpty: "[]",
	},
	"ListCommands": {
		call: func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) {
			return c.ListCommands(ctx, "", "10", "cid")
		},
		wantEmpty: `{"Commands":[]}`,
	},
	// ListPlugins returns a typed slice rather than raw JSON, so it is adapted
	// here to keep one matrix authoritative; a nil slice marshals to "null".
	"ListPlugins": {
		call: func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) {
			ps, err := c.ListPlugins(ctx)
			if err != nil {
				return nil, err
			}
			return json.Marshal(ps)
		},
		wantEmpty: "null",
	},
	"CancelCommands": {
		call: func(ctx context.Context, c *FormaeClient) (json.RawMessage, error) {
			return c.CancelCommands(ctx, "", "cid")
		},
		wantEmpty: `{"CommandIds":[]}`,
	},
}

func notFoundServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClassic404OnACollectionStaysAnEmptyList(t *testing.T) {
	for name, tc := range collectionCalls {
		t.Run(name, func(t *testing.T) {
			c := newTestFormaeClient(notFoundServer(t))

			got, err := tc.call(context.Background(), c)
			if err != nil {
				t.Fatalf("%s: a classic 404 must still read as empty: %v", name, err)
			}
			if string(got) != tc.wantEmpty {
				t.Errorf("%s: got %s, want %s", name, got, tc.wantEmpty)
			}
		})
	}
}

func TestHosted404OnACollectionIsARoutingError(t *testing.T) {
	for name, tc := range collectionCalls {
		t.Run(name, func(t *testing.T) {
			c := newTestHostedClient(t, notFoundServer(t), "Bearer live-token", nil)

			got, err := tc.call(context.Background(), c)
			if err == nil {
				t.Fatalf("%s: a hosted 404 must not read as empty, got %s", name, got)
			}
			if !strings.Contains(err.Error(), "did not route") {
				t.Errorf("%s: the error should say the request was not routed: %v", name, err)
			}
			if !strings.Contains(err.Error(), testInstallation) {
				t.Errorf("%s: the error should name the installation it addressed: %v", name, err)
			}
		})
	}
}

// GetCommandStatus is deliberately not in that set. Its 404 is an
// endpoint-specific "no such command" and keeps its meaning under both modes.
// Telling the two apart properly needs a stable edge error envelope, which does
// not exist yet, so the narrowing stops where the ambiguity starts.
func TestCommandStatus404KeepsItsMeaning(t *testing.T) {
	t.Run("classic", func(t *testing.T) {
		c := newTestFormaeClient(notFoundServer(t))

		_, err := c.GetCommandStatus(context.Background(), "cmd-1", "cid")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("want a command-not-found error, got %v", err)
		}
	})

	// Under hosted the same 404 has two readings and the status cannot separate
	// them, so it reports both rather than asserting the one that sends the
	// reader hunting for a command that was never the problem.
	t.Run("hosted", func(t *testing.T) {
		c := newTestHostedClient(t, notFoundServer(t), "Bearer live-token", nil)

		_, err := c.GetCommandStatus(context.Background(), "cmd-1", "cid")
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "cmd-1 was not found") {
			t.Errorf("the object reading must survive: %v", err)
		}
		if !strings.Contains(err.Error(), "did not route") {
			t.Errorf("the routing reading must be offered too: %v", err)
		}
	})
}

// An installation can disappear underneath a live session: a trial ends, a
// subscription lapses, someone suspends or destroys it. Sessions stay open for
// days, so this is an ordinary event rather than an exotic one, and when it
// happens every call fails at once. Each of them has to say the same true
// thing rather than three different misleading ones.
func TestEveryCallExplainsAGoneInstallation(t *testing.T) {
	srv := notFoundServer(t)
	c := newTestHostedClient(t, srv, "Bearer live-token", nil)
	ctx := context.Background()

	calls := map[string]func() error{
		"ListResources":  func() error { _, e := c.ListResources(ctx, ""); return e },
		"ListStacks":     func() error { _, e := c.ListStacks(ctx); return e },
		"ListTargets":    func() error { _, e := c.ListTargets(ctx, ""); return e },
		"ListPolicies":   func() error { _, e := c.ListPolicies(ctx); return e },
		"ListGenerators": func() error { _, e := c.ListGenerators(ctx); return e },
		"ListPlugins":    func() error { _, e := c.ListPlugins(ctx); return e },
		"ListCommands":   func() error { _, e := c.ListCommands(ctx, "", "10", "cid"); return e },
		"CancelCommands": func() error { _, e := c.CancelCommands(ctx, "", "cid"); return e },
		"GetCommandStatus": func() error {
			_, e := c.GetCommandStatus(ctx, "cmd-1", "cid")
			return e
		},
		"CheckHealth":   func() error { return c.CheckHealth(ctx) },
		"GetAgentStats": func() error { _, e := c.GetAgentStats(ctx); return e },
		"ListChanges":   func() error { _, e := c.ListChangesSinceLastReconcile(ctx, "default"); return e },
		"SubmitCommand": func() error {
			_, e := c.SubmitCommand(ctx, "apply", "reconcile", false, false, []byte("{}"), "cid")
			return e
		},
		"DestroyByQuery": func() error { _, e := c.DestroyByQuery(ctx, "stack:x", false, "cid"); return e },
		"ForceSync":      func() error { return c.ForceSync(ctx) },
		"ForceDiscover":  func() error { return c.ForceDiscover(ctx) },
		"ForceCheckTTL":  func() error { _, e := c.ForceCheckTTL(ctx); return e },
		"ForceReconcile": func() error { _, _, e := c.ForceReconcileStack(ctx, "default"); return e },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("a gone installation must not read as success")
			}
			if !strings.Contains(err.Error(), testInstallation) {
				t.Errorf("must name the installation that could not be reached: %v", err)
			}
			if !strings.Contains(err.Error(), "subscription") {
				t.Errorf("must offer the reason a reader can act on: %v", err)
			}
			// The edge's body is not an explanation and must not be pasted in.
			if strings.Contains(err.Error(), "404 page not found") {
				t.Errorf("the endpoint's body reached the caller: %v", err)
			}
		})
	}
}

// The same 404 against a self-hosted agent keeps meaning what the endpoint says
// it means. Only the shared edge is ambiguous.
func TestClassicKeepsItsOwn404Meanings(t *testing.T) {
	c := newTestFormaeClient(notFoundServer(t))
	ctx := context.Background()

	if _, err := c.GetCommandStatus(ctx, "cmd-1", "cid"); err == nil ||
		!strings.Contains(err.Error(), "command cmd-1 not found") ||
		strings.Contains(err.Error(), "subscription") {
		t.Errorf("a classic command lookup must report the command, nothing more: %v", err)
	}
	if err := c.CheckHealth(ctx); err == nil || !strings.Contains(err.Error(), "unhealthy status: 404") {
		t.Errorf("classic health keeps its own wording: %v", err)
	}
}
