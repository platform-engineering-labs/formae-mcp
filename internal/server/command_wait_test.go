package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// A caller that wants to know how a command ended should not have to poll from
// the outside. Before this, an agent driving an apply improvised a wait by
// shelling out to `sleep 5` between status calls, which is both slow and
// visible to the user as a series of pointless commands.
//
// The wait needs exactly two judgments: which state a payload reports, and
// whether that state is one it can stop on.
func TestCommandStateFromStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "a command in flight",
			raw:  `{"Commands":[{"CommandId":"abc","State":"InProgress"}]}`,
			want: "InProgress",
		},
		{
			name: "a finished command",
			raw:  `{"Commands":[{"CommandId":"abc","State":"Success"}]}`,
			want: "Success",
		},
		{
			name: "a failed command",
			raw:  `{"Commands":[{"CommandId":"abc","State":"Failed"}]}`,
			want: "Failed",
		},
		{
			// An empty list is not a state. Reporting "" keeps the wait going
			// rather than treating an unreadable answer as an ending, which
			// would report a command as finished that nobody has looked at.
			name: "no commands in the payload",
			raw:  `{"Commands":[]}`,
			want: "",
		},
		{
			name: "a payload this build cannot read",
			raw:  `{"something":"else"}`,
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandStateFromStatus(json.RawMessage(tc.raw)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Canceled is terminal: a caller waiting on a command someone cancelled must
// be released, not left polling until its own timeout.
func TestIsTerminalCommandState(t *testing.T) {
	for state, want := range map[string]bool{
		"Success":    true,
		"Failed":     true,
		"Canceled":   true,
		"InProgress": false,
		"":           false,
		"Nonsense":   false,
	} {
		if got := isTerminalCommandState(state); got != want {
			t.Errorf("state %q: got %v, want %v", state, got, want)
		}
	}
}

// The loop must stop on the first terminal answer, not poll a fixed number of
// times, and it must hand back the payload it stopped on.
func TestWaitForCommand_StopsOnTheFirstEnding(t *testing.T) {
	calls := 0
	fetch := func(context.Context) (json.RawMessage, error) {
		calls++
		if calls < 3 {
			return json.RawMessage(`{"Commands":[{"State":"InProgress"}]}`), nil
		}
		return json.RawMessage(`{"Commands":[{"State":"Success"}]}`), nil
	}

	raw, state, err := waitForCommand(context.Background(), time.Minute, fetch)
	if err != nil {
		t.Fatalf("waitForCommand: %v", err)
	}
	if state != "Success" {
		t.Errorf("got state %q, want Success", state)
	}
	if calls != 3 {
		t.Errorf("polled %d times, want 3", calls)
	}
	if !strings.Contains(string(raw), "Success") {
		t.Errorf("returned payload is not the terminal one: %s", raw)
	}
}

// Running out of budget is a report, not a failure: the caller gets the latest
// status and can decide whether to keep waiting. Returning an error here would
// make a slow command indistinguishable from a broken one.
func TestWaitForCommand_TimeoutReturnsTheLatestStatus(t *testing.T) {
	fetch := func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"Commands":[{"State":"InProgress"}]}`), nil
	}

	raw, state, err := waitForCommand(context.Background(), time.Millisecond, fetch)
	if err != nil {
		t.Fatalf("a timeout must not be an error: %v", err)
	}
	if state != "InProgress" {
		t.Errorf("got state %q, want InProgress", state)
	}
	if !strings.Contains(string(raw), "InProgress") {
		t.Errorf("no status returned on timeout: %s", raw)
	}
}

// A caller's cancellation has to win, or a tool call outlives the request that
// made it.
func TestWaitForCommand_HonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fetch := func(context.Context) (json.RawMessage, error) {
		cancel()
		return json.RawMessage(`{"Commands":[{"State":"InProgress"}]}`), nil
	}

	if _, _, err := waitForCommand(ctx, time.Minute, fetch); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestResolveWaitTimeout(t *testing.T) {
	if got := resolveWaitTimeout(0); got != defaultCommandWaitTimeout {
		t.Errorf("zero should default, got %v", got)
	}
	if got := resolveWaitTimeout(-5); got != defaultCommandWaitTimeout {
		t.Errorf("negative should default, got %v", got)
	}
	if got := resolveWaitTimeout(30); got != 30*time.Second {
		t.Errorf("got %v, want 30s", got)
	}
	if got := resolveWaitTimeout(99999); got != maxCommandWaitTimeout {
		t.Errorf("oversized should cap, got %v", got)
	}
}
