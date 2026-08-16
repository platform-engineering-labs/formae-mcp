package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The six endpoints that translate a 404 into an empty payload. Under hosted
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
			if !strings.Contains(err.Error(), "routing") {
				t.Errorf("%s: the error should name routing as the likely cause: %v", name, err)
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

	t.Run("hosted", func(t *testing.T) {
		c := newTestHostedClient(t, notFoundServer(t), "Bearer live-token", nil)

		_, err := c.GetCommandStatus(context.Background(), "cmd-1", "cid")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("want a command-not-found error, got %v", err)
		}
		if strings.Contains(err.Error(), "routing") {
			t.Fatalf("an object-not-found must not be reported as a routing failure: %v", err)
		}
	})
}
