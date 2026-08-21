package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Setup is prose, and prose is mostly not testable. What is testable is the
// small number of orderings the file itself insists on, each of which is there
// because getting it wrong produces a failure that reads like the user's machine
// rather than like the skill.
//
// Deliberately not tested: that every tool a skill names is registered. The
// obvious version of that check cannot tell a tool name from a result field
// name, since both are backticked snake_case, and the exclusion list it needs
// grows with every field any skill documents. A test carrying a deny-list that
// long stops being read.
func TestSetupSkill_HoldsItsOwnOrdering(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "skills", "setup", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)

	// Every step before the health check must avoid agent-backed tools, or the
	// one skill whose job is the no-profile machine trips the no-profile gate.
	health := strings.LastIndex(text, "check_health")
	if health < 0 {
		t.Fatal("setup never checks the agent")
	}
	for _, agentBacked := range []string{
		"list_resources", "list_stacks", "apply_forma", "get_agent_stats",
	} {
		if i := strings.Index(text, agentBacked); i >= 0 && i < health {
			t.Errorf("setup reaches %q before the agent is known to be reachable, "+
				"which is what trips the no-profile gate", agentBacked)
		}
	}

	// The console link has to carry the marker, or the console closes the user's
	// signup by telling them to run the command they are already inside.
	if !strings.Contains(text, "?from=mcp") {
		t.Error("setup's console link is unmarked, so the console will close it with /formae:setup")
	}

	// And marking the trip out is worthless if the trip back is not written
	// down: the console now points the reader at their harness, so the skill
	// owes them something to do when they get there.
	console := strings.LastIndex(text, "console.formae.ai")
	if console < 0 {
		t.Fatal("setup never names the console")
	}
	if !strings.Contains(text[console:], "login") {
		t.Error("setup ends at the console, leaving the user with nothing to come back to")
	}

	// complete_login after a login that already finished is an error, and the
	// return leg is exactly where a login normally finishes on its own (the
	// session is still open, so it reports already-signed-in and syncs). The
	// skill must say the second call is conditional rather than instruct it
	// outright, which is the defect review caught here.
	if !strings.Contains(text[console:], "Only call `complete_login` if") {
		t.Error("the return leg does not make complete_login conditional; " +
			"an already-signed-in login leaves nothing for it to complete")
	}
}
