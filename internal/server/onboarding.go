package server

import (
	"errors"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/profile"
)

// Before an agent-backed tool resolves anything, the MCP asks what this machine
// holds — and it asks with a read that creates nothing.
//
// The reason is that `formae connection resolve` bootstraps. On a machine with
// no profile it writes a classic localhost default and points the active pointer
// at it, so by the time the MCP could observe "nothing configured" it has already
// answered the question on the user's behalf, in favour of self-hosting, for
// someone who may have come here to use the hosted platform. The act of looking
// destroys the answer.
//
// The gate lives here rather than in a skill because the case it exists for is a
// user reaching for some other tool entirely, with no skill in the loop.
//
// Nothing is remembered. A user who says "hosted" and goes away is asked again on
// their next call, which is accepted: the state is cheap to re-derive, and
// remembering it would mean session state whose only job is to suppress a prompt.

// consoleURL is the hosted console, and consoleFromMCP is the link this server
// hands out.
//
// The query parameter marks the visit as coming from an MCP server. The console
// reads it and stops closing the signup with "now run /formae:setup", because a
// reader who arrived by this link already did: that command is what sent them.
// Instead it points them back at the harness session they came from.
//
// The two are separate constants so the marker is written once, and every
// message that sends someone to the console reaches for the marked one. A test
// pins that no bare origin survives in the instruction, because an edit that
// reached for consoleURL would drop the marker silently and the console would go
// back to telling people to do the thing they just did.
const (
	consoleURL     = "https://console.formae.ai"
	consoleFromMCP = consoleURL + "/?from=mcp"
)

// ErrNoProfile is a machine with nothing configured. Its message is the
// instruction a caller acts on rather than a description of the problem, in the
// same idiom as config.AmbiguousProfileError.
//
// The self-hosted branch names use_profile and deliberately not create_profile
// with force. `formae profile use` goes through the store's initialization, which
// never deletes a profile file and never overwrites an existing one; a forced
// create removes its destination first, and on a store whose configuration is a
// legacy formae.conf.pkl that destination is the file initialization has just
// migrated the user's own configuration into.
//
// The hosted branch has to be self-sufficient. Codex and OpenCode install this
// MCP server without the skills, so for those harnesses this message is the only
// onboarding instruction that exists and it cannot assume /formae:setup is
// invocable.
//
// It signs in before it mentions the console, which is the opposite of what it
// used to do, for two reasons. An invited user has their invite claimed at their
// first sign-in and finishes it with grants over an org that already has an
// agent, so sending them to the console is a wasted trip. And a user who really
// does need one cannot be told to go there and left: the console now closes by
// pointing them back at their harness, so the harness has to have something to
// do when they return. Signing in again is that something, and it costs no
// second browser hop: the sync runs after an already-authenticated sign-in just
// as it does after a completed flow, and it is what writes the profile the newly
// provisioned agent earns them.
type ErrNoProfile struct{}

func (*ErrNoProfile) Error() string {
	return "No formae configuration exists on this machine, so formae does not know whether you are " +
		"running your own agent or using the hosted platform, and it will not guess. Ask the user which, then:\n" +
		"  - self-hosted: call use_profile with name \"default\", then retry. Afterwards mention " +
		"`formae profile edit` for pointing it at their agent, and offer to help author a forma.\n" +
		"  - hosted: call the login tool to sign in. If the sign-in writes no profiles they have no agent " +
		"yet: send them to " + consoleFromMCP + " to create an organization and provision one, wait until " +
		"they say it is done, then call login again to pick it up."
}

// ErrNoActiveProfile is configuration with nothing usable selected: profiles but
// no pointer, or a pointer naming a profile that is gone. Both are ordinary —
// deleting the profile the pointer names reaches the second — and in both,
// resolution fails. Reporting it here keeps the CLI's good message from being
// replaced by a generic connection error.
type ErrNoActiveProfile struct {
	Profiles []string
}

func (e *ErrNoActiveProfile) Error() string {
	if len(e.Profiles) == 0 {
		return "This machine has formae configuration but no usable active profile. " +
			"Ask the user which profile to use and call use_profile with it."
	}
	return fmt.Sprintf(
		"This machine has profiles but none is selected, so formae cannot tell which one you meant. "+
			"Ask the user which to use and call use_profile with it: %s",
		strings.Join(e.Profiles, ", "))
}

// gateStore reports the instruction a caller needs when this machine cannot
// resolve a connection, or nil when it can.
//
// A failure to read the store is deliberately not a refusal. It is not evidence
// that nothing is configured, and treating it as such would send a user with a
// working setup through onboarding on the strength of a permissions error;
// resolution is left to fail and say what actually happened.
func gateStore() error {
	state, names, err := profile.State()
	if err != nil {
		return nil
	}
	switch state {
	case profile.Unconfigured:
		return &ErrNoProfile{}
	case profile.NoActive:
		return &ErrNoActiveProfile{Profiles: names}
	default:
		return nil
	}
}

// explainLapsedSession turns a resolution failure into the instruction to sign in,
// when signing in is what would fix it.
//
// The signal already exists and is already decoded: `connection resolve` reports
// auth_failed with the auth plugin's own code, and slice 2b's decoder validates
// it against the declared set. So this renders an answer the MCP already has
// rather than probing a second time — a separate liveness check would be a second
// answer to a question already answered, and the two could disagree.
//
// Only not_logged_in and session_expired are offered a sign-in. issuer_unreachable
// and unsupported are not fixed by one, and sending a user through a browser flow
// that cannot help is worse than saying what happened.
func explainLapsedSession(err error) error {
	var re *config.ResolveError
	if !errors.As(err, &re) || re.Code != "auth_failed" {
		return err
	}
	switch re.PluginCode {
	case "not_logged_in", "session_expired":
		return fmt.Errorf("%w. Call the login tool to sign in again, then retry", err)
	default:
		return err
	}
}
