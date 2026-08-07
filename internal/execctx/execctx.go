// Package execctx resolves the immutable per-call execution context. Resolve it
// once at the top of a handler and thread it through eval, extract, feature
// gates, and the agent client — never re-read the active-profile pointer or
// re-resolve the profile partway through a call.
package execctx

import (
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
)

// Context is the frozen decision set for one MCP call.
type Context struct {
	ProfileName   string
	Mode          formaebin.Mode
	URL           string
	Port          string
	Installation  string // empty in classic mode
	CredentialRef string // empty in classic mode
	FormaeBin     string
}

// Resolver builds Contexts. Fields are function-typed so tests can inject.
type Resolver struct {
	endpoint func(profileName string) (url, port string, err error)
	bin      formaebin.BinResolver
}

// NewResolver wires a Resolver to config-backed endpoint resolution.
func NewResolver(bin formaebin.BinResolver) *Resolver {
	return &Resolver{endpoint: config.AgentEndpoint, bin: bin}
}

// Resolve produces the context for an optional profile name. This phase always
// resolves classic mode; a later phase adds hosted detection here (and only
// here), which is why every consumer must read Mode from the returned Context
// rather than assuming classic.
func (r *Resolver) Resolve(profileName string) (Context, error) {
	url, port, err := r.endpoint(profileName)
	if err != nil {
		return Context{}, err
	}
	mode := formaebin.Classic
	return Context{
		ProfileName: profileName,
		Mode:        mode,
		URL:         url,
		Port:        port,
		FormaeBin:   r.bin.Resolve(mode),
	}, nil
}
