// Package execctx resolves the immutable per-call execution context. Resolve it
// once at the top of a handler and thread it through eval, extract, feature
// gates, and the agent client — never re-read the active-profile pointer or
// re-resolve the profile partway through a call.
package execctx

import (
	"context"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

// Context is the frozen decision set for one MCP call.
type Context struct {
	// ProfileName is the effective profile the CLI reported. Empty only when no
	// profile exists at all.
	ProfileName string
	// Conn is where this call sends agent requests.
	Conn config.Connection
	// Credential authenticates this call, and is the zero value for classic.
	//
	// It sits beside the connection rather than inside the hosted arm on
	// purpose: configuration gets logged, compared and rendered, and a secret
	// inside it would ride along on all three. The combination this leaves
	// representable — a classic connection with a credential — is ruled out
	// where it would matter: the decoder refuses that shape, and the client
	// holds a credential only in the hosted arm of its routing.
	Credential secret.Value
	// FormaeBin is the formae binary this call shells out to. Resolved once;
	// callers take it from here rather than resolving again.
	FormaeBin string
}

// Resolver builds Contexts. The resolve func is injectable so tests do not
// shell out to a CLI.
type Resolver struct {
	resolve func(ctx context.Context, bin, profileName string, forceRefresh bool) (config.Resolved, error)
	bin     formaebin.BinResolver
}

// NewResolver wires a Resolver to CLI-backed configuration resolution.
func NewResolver(bin formaebin.BinResolver) *Resolver {
	return &Resolver{resolve: config.Resolve, bin: bin}
}

// Bin returns the resolved formae binary path for callers that need one without
// a full profile resolution (local planning that never reaches the agent).
func (r *Resolver) Bin() string { return r.bin.Resolve() }

// Managed reports whether the resolved formae is the copy the launcher
// provisioned into the user's own tree, and can therefore be upgraded without
// sudo.
func (r *Resolver) Managed() bool { return r.bin.Managed() }

// Resolve produces the context for an optional profile name. The CLI is the
// configuration authority: it reports which profile it actually used, so this
// package never reasons about what "active" meant at the time of the call.
//
// forceRefresh is for the 401 path, which re-resolves with a fresh credential
// and then checks the target did not move.
func (r *Resolver) Resolve(ctx context.Context, profileName string, forceRefresh bool) (Context, error) {
	bin := r.bin.Resolve()
	res, err := r.resolve(ctx, bin, profileName, forceRefresh)
	if err != nil {
		return Context{}, err
	}
	return Context{
		ProfileName: res.Profile,
		Conn:        res.Conn,
		Credential:  res.Credential,
		FormaeBin:   bin,
	}, nil
}
