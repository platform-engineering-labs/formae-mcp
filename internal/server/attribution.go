package server

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

// reach is how far one call got toward its destination.
//
// Only the request executor may advance it. Deriving it from IsError, or from
// the context having resolved, is how it stops being true: a forma file that
// fails to evaluate has resolved a destination and contacted nothing, and an
// agent 500 is a failure that was nonetheless answered.
type reach int

const (
	// reachResolved: a destination is known and nothing was sent.
	reachResolved reach = iota
	// reachAttempted: bytes left, and the outcome is unknown.
	reachAttempted
	// reachAnswered: the agent responded, whatever the status.
	reachAnswered
)

// destination is what a result says about where the call went. The zero value
// means no destination was ever resolved, which is the honest answer for a
// failure that happened before resolution.
type destination struct {
	ec    execctx.Context
	reach reach
}

// resolved names a destination for a call that has not built a client yet.
func resolved(ec execctx.Context) destination {
	return destination{ec: ec, reach: reachResolved}
}

// reached names a destination for a call that has a client, taking how far it
// got from the client itself rather than guessing.
func reached(ec execctx.Context, c *FormaeClient) destination {
	if c == nil {
		return resolved(ec)
	}
	return destination{ec: ec, reach: c.reach}
}

// note renders the attribution line, or "" when there is nothing true to say.
//
// Only hosted calls carry one. A classic connection addresses the agent the
// user pointed at, which they already know; a hosted one addresses a single
// installation behind an endpoint shared with every other installation, and a
// profile name can later be repointed, so the name alone is weak evidence.
//
// The wording differs by reach because the three states are different claims.
// Saying an installation was addressed when nothing was sent would send an
// operator to check for work that cannot exist, which is worse than saying
// nothing at all.
func (d destination) note() string {
	hosted, ok := d.ec.Conn.(config.Hosted)
	if !ok {
		return ""
	}
	switch d.reach {
	case reachAnswered:
		// "The hosted endpoint answered", not "the installation answered".
		// Every response arrives from the shared edge, and the edge answers for
		// itself when it cannot route: a hosted 404 on a collection is reported
		// two lines away as a routing miss, so claiming the installation had
		// answered would contradict it in the same result. Telling the two
		// apart needs a stable edge error envelope, which does not exist yet.
		return fmt.Sprintf("The hosted endpoint answered for installation %s, via profile %q.",
			hosted.Installation, d.ec.ProfileName)
	case reachAttempted:
		return fmt.Sprintf(
			"This request was sent for installation %s via profile %q and no response came back, "+
				"so its outcome is unknown: it may already have taken effect.",
			hosted.Installation, d.ec.ProfileName)
	default:
		return fmt.Sprintf("Profile %q resolves to installation %s; nothing was sent.",
			d.ec.ProfileName, hosted.Installation)
	}
}

// attribute appends the attribution to a result as its own content block.
//
// A separate block, never a wrapper or a prefix: the first block is the agent's
// payload and a consumer parses it, so changing its content would change the
// result schema. The version-skew notice already works this way.
func attribute(d destination, res *mcp.CallToolResult) *mcp.CallToolResult {
	note := d.note()
	if note == "" {
		return res
	}
	res.Content = append(res.Content, &mcp.TextContent{Text: note})
	return res
}
