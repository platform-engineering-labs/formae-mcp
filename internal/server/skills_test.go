package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registeredToolNames is every tool this server exposes, read through the
// protocol rather than off the registration code, so the test sees what a model
// would see.
func registeredToolNames(t *testing.T) map[string]bool {
	t.Helper()
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	return names
}

// A skill names tools in prose, so nothing but a test connects the two: renaming
// a tool leaves the skill telling a model to call something that no longer
// exists, and the failure surfaces as a confused conversation rather than as a
// build error.
//
// This pins only the tools the setup skill's steps depend on, which is the skill
// this work rewrote.

// setupSkill returns the setup skill's text.
func setupSkill(t *testing.T) string {
	t.Helper()
	// The test binary runs in the package directory.
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "setup", "SKILL.md"))
	if err != nil {
		t.Fatalf("read the setup skill: %v", err)
	}
	return string(data)
}

func TestSetupSkill_NamesOnlyToolsThatExist(t *testing.T) {
	text := setupSkill(t)
	registered := registeredToolNames(t)

	for _, name := range []string{"list_profiles", "use_profile", "login", "complete_login", "check_health"} {
		if !strings.Contains(text, name) {
			t.Errorf("the setup skill no longer mentions %q; if the step was removed, remove it here too", name)
			continue
		}
		if !registered[name] {
			t.Errorf("the setup skill tells a model to call %q, which is not a registered tool", name)
		}
	}
}

// Steps 1 to 5 must reach no agent-backed tool, or the skill trips the very
// no-profile prompt it exists to resolve. check_health is the first that does,
// and it has to stay last.
func TestSetupSkill_ReachesTheAgentOnlyAtTheEnd(t *testing.T) {
	text := setupSkill(t)

	health := strings.Index(text, "check_health")
	if health < 0 {
		t.Fatal("the setup skill never checks the agent")
	}

	// Every agent-backed tool named before check_health would be resolved, and
	// resolution is what the gate stops on an unconfigured machine.
	for _, agentBacked := range []string{"list_stacks", "list_resources", "list_targets", "get_agent_stats", "apply_forma"} {
		if at := strings.Index(text, agentBacked); at >= 0 && at < health {
			t.Errorf("the setup skill calls %q before check_health; on an unconfigured machine "+
				"that reaches resolution and trips the onboarding gate", agentBacked)
		}
	}

	// The first mention of check_health must come after the profile steps.
	profiles := strings.Index(text, "list_profiles")
	if profiles < 0 || profiles > health {
		t.Error("the setup skill checks the agent before it knows what the machine holds")
	}
}

// The self-hosted branch must not tell anyone to force a profile creation: a
// forced create removes its destination first, and on a store whose configuration
// is a legacy formae.conf.pkl that destination is the file the CLI has just
// migrated the user's own configuration into.
func TestSetupSkill_DoesNotOfferAForcedCreate(t *testing.T) {
	text := setupSkill(t)
	if strings.Contains(text, "create_profile") {
		t.Error("the setup skill offers create_profile; use_profile is the one that cannot destroy a legacy config")
	}
	if strings.Contains(text, "force: true") || strings.Contains(text, "force=true") {
		t.Error("the setup skill offers a forced write")
	}
}

// Setup must not ask hosted-or-self-hosted. It is advertised in exactly one
// place — the hosted console, which tells the user to run it after creating an
// account — so someone running it has already answered that question with their
// last click. The gate asks it instead, where a user reaching for any other tool
// genuinely could be either.
func TestSetupSkill_AssumesHostedRatherThanAsking(t *testing.T) {
	text := setupSkill(t)

	if !strings.Contains(text, "assumes hosted") {
		t.Error("the setup skill no longer states that it assumes hosted")
	}

	// The question belongs in the gate, not here.
	for _, asking := range []string{
		"Are you using the **hosted** formae platform, or running **your own agent**?",
		"hosted or self-hosted?",
	} {
		if strings.Contains(text, asking) {
			t.Errorf("the setup skill asks the user which platform they are on: %q", asking)
		}
	}

	// The gate still asks, because there the ambiguity is real.
	if !strings.Contains((&ErrNoProfile{}).Error(), "hosted") {
		t.Error("the onboarding gate no longer offers the hosted branch")
	}
}

// A sign-in whose profile sync failed must not be reported as a failed sign-in:
// the session is saved and signing in again fixes nothing.
func TestLoginFailure_SyncIncompleteDoesNotReadAsAFailedSignIn(t *testing.T) {
	msg := describeLoginFailure("sync_incomplete", "")

	if !strings.Contains(msg, "signed in") {
		t.Errorf("the message does not tell the user they are signed in: %s", msg)
	}
	if strings.Contains(msg, "did not complete") {
		t.Errorf("the message says the sign-in did not complete, which is the opposite of what happened: %s", msg)
	}
}
