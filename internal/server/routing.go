package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

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

	// unrouted reports the error for a 404 this connection cannot explain as an
	// object simply being absent, or nil when it can.
	unrouted() error

	// scrub removes any credential this connection has sent from bytes the far
	// end returned, before they can reach a result, an error, or a log.
	scrub(body []byte) []byte

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

// scrub does nothing: the MCP sends a self-hosted agent no credential, so a
// response cannot be quoting one.
func (classicRoute) scrub(body []byte) []byte { return body }

// unrouted reports nothing. A self-hosted agent answers for itself, so its 404
// means whatever the endpoint says a 404 means.
func (classicRoute) unrouted() error { return nil }

// refresh does nothing. The MCP sends a self-hosted agent no credential, so
// there is nothing a second attempt would do differently.
func (classicRoute) refresh(context.Context) (bool, error) { return false, nil }

// hostedRoute addresses one installation behind the shared edge.
type hostedRoute struct {
	endpoint     string
	installation string
	credential   secret.Value
	refreshFn    refresher
	// used is every credential this route has put on the wire, kept so a
	// response quoting one can be scrubbed. It is bounded by one refresh.
	used []string
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
	sent := r.credential.Reveal()
	h.Set("Authorization", sent)
	if !slices.Contains(r.used, sent) {
		r.used = append(r.used, sent)
	}
}

// collectionMiss refuses to report a routing failure as an empty list. The
// shared edge answers 404 for an unknown or unrouted installation, and "no
// resources" would hide that behind a plausible answer.
func (r *hostedRoute) collectionMiss(json.RawMessage) (json.RawMessage, error) {
	return nil, fmt.Errorf("%w; reporting this as an empty result would hide it", r.unrouted())
}

// unrouted reports that the shared endpoint may not have reached this
// installation at all.
//
// The edge answers 404 for an installation it cannot route — one that has been
// suspended, destroyed, or reaped when a trial or subscription ended — and that
// is indistinguishable from any other 404 without an edge error envelope, which
// does not exist. It happens on an ordinary day: a session left open for days
// outlives the installation it was working against.
//
// So this never asserts a cause. Callers that could legitimately receive a 404
// report both possibilities; callers whose endpoint has no such reading report
// this alone.
func (r *hostedRoute) unrouted() error {
	return fmt.Errorf(
		"the hosted endpoint did not route this request to installation %s, which happens when an "+
			"installation is suspended, destroyed, or no longer covered by a subscription", r.installation)
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

// scrub removes any credential this route has used from bytes the far end
// sent back.
//
// The far end is trusted to route, not to be careful. An error page from an
// intermediary that echoes request headers would otherwise put the bearer
// token into a tool result and from there into a model's context and a
// transcript — a copy of the credential somewhere it can never be withdrawn
// from. Both the current and the previous credential are scrubbed, because a
// retry's response can still be quoting the request that failed.
func (r *hostedRoute) scrub(body []byte) []byte {
	for _, used := range r.used {
		if used == "" {
			continue
		}
		body = bytes.ReplaceAll(body, []byte(used), []byte(secret.Mask))
	}
	return body
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
