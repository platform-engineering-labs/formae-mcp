package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

// installationHeader routes a request to one hosted installation. The edge
// selects a backend on this alone, which is why it is set exactly once.
const installationHeader = "Formae-Installation"

// refresher re-resolves this call's profile with the credential refreshed. It
// is injected rather than reached for directly, so the credential a retry uses
// comes from the same seam the first resolution did.
type refresher func(ctx context.Context) (execctx.Context, error)

// routing is how one connection addresses and authenticates its requests.
//
// The address lives here with the credential rather than beside them on the
// client: a hosted route holding a classic endpoint, or a classic route holding
// the hosted one, is then not a thing that can be written down.
type routing interface {
	// url builds the request URL for a path and query.
	url(path string, q url.Values) string

	// decorate adds the headers this connection requires.
	decorate(h http.Header)

	// collectionMiss answers a 404 from an endpoint that lists things.
	collectionMiss(empty json.RawMessage) (json.RawMessage, error)

	// refresh re-resolves the credential and reports whether anything changed,
	// so the caller knows whether a second attempt could differ from the first.
	refresh(ctx context.Context) (bool, error)
}

// classicRoute addresses a self-hosted agent.
type classicRoute struct {
	base string
}

func (r classicRoute) url(path string, q url.Values) string { return joinURL(r.base, path, q) }

func (classicRoute) decorate(http.Header) {}

func (classicRoute) collectionMiss(empty json.RawMessage) (json.RawMessage, error) {
	return empty, nil
}

// refresh does nothing. The MCP sends a self-hosted agent no credential, so
// there is nothing a second attempt would do differently.
func (classicRoute) refresh(context.Context) (bool, error) { return false, nil }

// hostedRoute addresses one installation behind the shared edge.
type hostedRoute struct {
	endpoint     string
	installation string
	credential   secret.Value
	refreshFn    refresher
}

// String and GoString mask, and holding a secret.Value is not enough on its own
// to make that true.
//
// fmt reaches a field's Format or String method only through reflect.Value's
// Interface, which it cannot call on an *unexported* field. So a struct with an
// unexported secret.Value is printed reflectively, field by field, and the
// credential comes straight out — which is exactly what a developer sees when
// they print the routing while debugging the thing that holds it. Any type
// keeping a credential in an unexported field has to mask itself.
func (r *hostedRoute) String() string {
	return fmt.Sprintf("hosted{endpoint:%s installation:%s credential:%s}",
		r.endpoint, r.installation, r.credential)
}

func (r *hostedRoute) GoString() string { return r.String() }

func (r *hostedRoute) url(path string, q url.Values) string { return joinURL(r.endpoint, path, q) }

// decorate sets the routing header with Set and never Add. The edge rejects a
// duplicated header after it has already selected a router, so a second value
// is not a warning, it is a failure that surfaces somewhere else entirely.
func (r *hostedRoute) decorate(h http.Header) {
	h.Set(installationHeader, r.installation)
	h.Set("Authorization", r.credential.Reveal())
}

// collectionMiss refuses to report a routing failure as an empty list. The
// shared edge answers 404 for an unknown or unrouted installation, and "no
// resources" would hide that behind a plausible answer.
func (r *hostedRoute) collectionMiss(json.RawMessage) (json.RawMessage, error) {
	return nil, fmt.Errorf(
		"the hosted endpoint did not route this request to installation %s; "+
			"this is more likely a routing problem than an empty result",
		r.installation)
}

// refresh re-resolves with the credential refreshed and refuses to let the
// target move.
//
// The comparison is the point. Without it the refresh path silently
// re-introduces the skew that resolving configuration and credentials together
// exists to prevent, and a retry could read from a different installation than
// the one the call set out to address.
//
// The profile name is deliberately not compared: the target is the endpoint and
// the installation, and a pointer that moved while still resolving to the same
// installation has not moved the target.
func (r *hostedRoute) refresh(ctx context.Context) (bool, error) {
	if r.refreshFn == nil {
		return false, nil
	}
	next, err := r.refreshFn(ctx)
	if err != nil {
		return false, err
	}
	hosted, ok := next.Conn.(config.Hosted)
	if !ok {
		return false, errConnectionMoved
	}
	if hosted.Endpoint != r.endpoint || hosted.Installation != r.installation {
		return false, errConnectionMoved
	}
	if next.Credential.IsZero() {
		return false, errors.New("formae refreshed the connection but returned no credential")
	}
	r.credential = next.Credential
	return true, nil
}

// withEndpoint returns a copy addressing a different origin. It exists for
// tests, which need the validated hosted routing behaviour aimed at a local
// server rather than at the real edge.
func (r *hostedRoute) withEndpoint(endpoint string) *hostedRoute {
	copied := *r
	copied.endpoint = endpoint
	return &copied
}

var errConnectionMoved = errors.New(
	"the connection changed while this request was in flight, so it was abandoned rather than " +
		"retried against a different installation")

func joinURL(base, path string, q url.Values) string {
	u := base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// refuseRedirects is the hosted redirect policy: hand the 3xx back rather than
// follow it, so do can report it as a routing error.
//
// Every redirect is refused, not only a cross-origin one. The agent API has no
// redirect worth following, and refusing outright avoids getting origin
// equivalence subtly right for no benefit. Returning ErrUseLastResponse rather
// than an error keeps the status available and leaves nothing wrapped in a
// *url.Error for a caller to unwrap incorrectly.
func refuseRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
