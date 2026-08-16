package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
)

// request describes one agent API call. Every request is built here so that
// endpoint, headers, cancellation and error construction hold on all of them.
// A per-method audit is how one method ends up missing a header.
type request struct {
	Method      string
	Path        string
	Query       url.Values
	Headers     map[string]string
	Body        io.Reader
	ContentType string
}

// retryPolicy says whether a request may be sent a second time after its
// credential has been refreshed.
//
// It is an argument rather than a field on request, so a new call site cannot
// inherit an answer by leaving a zero value alone. Retryability is a property
// of the request and not of its verb: a simulated apply is a POST that may be
// safe, and a nominally read-only tool may issue a POST, so inferring it from
// the method or from a tool annotation would be guessing.
type retryPolicy int

const (
	// noRetry returns the original failure. Mutations use this: nothing
	// establishes that a 401 always precedes dispatch, and duplicating an
	// infrastructure mutation to save one error is the wrong trade.
	noRetry retryPolicy = iota
	// retryOnce resends once, and only once, after a successful refresh.
	retryOnce
)

// maxResponseBytes bounds one agent response.
const maxResponseBytes = 32 << 20

var errResponseTooLarge = errors.New("the agent returned more data than this build will read")

// errRetryableBody guards a combination that would corrupt a request silently.
var errRetryableBody = errors.New(
	"a retryable request may not carry a body: the first attempt consumes the reader, " +
		"so a retry would send an empty one")

// do executes a request against the agent and returns the body and status.
func (c *FormaeClient) do(ctx context.Context, r request, retry retryPolicy) ([]byte, int, error) {
	if retry == retryOnce && r.Body != nil {
		return nil, 0, errRetryableBody
	}

	body, status, err := c.send(ctx, r)
	if err != nil || status != http.StatusUnauthorized {
		return body, status, err
	}

	// A 401 has two distinct sources, and only one of them is actionable here.
	// A credential command that failed carries the plugin's own code and was
	// reported at resolution. This 401 arrived after a successful resolution,
	// so there is no plugin error behind it: it can mean expiry, wrong
	// audience, wrong issuer, or a malformed credential, and nothing here can
	// tell which. Refresh, and say no more than "unauthorized" if it recurs.
	//
	// 403 has no branch at all, deliberately: a denied installation or tenant
	// is an authorization failure, not an expired session.
	refreshed, rerr := c.route.refresh(ctx)
	if rerr != nil {
		return body, status, rerr
	}
	if !refreshed || retry != retryOnce {
		return body, status, nil
	}
	return c.send(ctx, r)
}

// send performs exactly one attempt.
func (c *FormaeClient) send(ctx context.Context, r request) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, c.route.url(r.Path, r.Query), r.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	if r.ContentType != "" {
		req.Header.Set("Content-Type", r.ContentType)
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	// The routing decorates last, so a caller cannot displace the routing
	// header or the credential by naming one in Headers.
	c.route.decorate(req.Header)

	// Attempted means bytes left this process, which is what "it may have
	// acted" rests on. Taken from the trace rather than from "we are about to
	// call Do", because a DNS or TLS failure sends nothing and telling an
	// operator to go and check would be a false alarm in the costly direction.
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				c.advance(reachAttempted)
			}
		},
	}))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	c.advance(reachAnswered)
	defer func() { _ = resp.Body.Close() }()

	// A 3xx only reaches here on a connection whose policy refuses to follow
	// redirects. Reporting it as routing rather than passing the status up
	// keeps a caller from treating a redirect body as an answer.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, resp.StatusCode, fmt.Errorf(
			"the hosted endpoint answered %d with a redirect, which is refused: "+
				"this is a routing problem, not a response", resp.StatusCode)
	}

	// Bounded, and scrubbed before it can reach a caller. The bound matches the
	// one the configuration oracle already applies to its subprocess: an
	// unbounded read from a peer is an unbounded allocation, and under hosted
	// that peer is remote and shared rather than a process on this machine.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, resp.StatusCode, errResponseTooLarge
	}
	return c.route.scrub(body), resp.StatusCode, nil
}

// advance raises the high-water mark, never lowers it: a call that answered
// once has answered, whatever a later attempt does.
func (c *FormaeClient) advance(r reach) {
	if r > c.reach {
		c.reach = r
	}
}
