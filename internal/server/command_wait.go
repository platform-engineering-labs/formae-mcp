package server

import (
	"context"
	"encoding/json"
	"time"
)

// Command states the agent reports. Three of the four are endings.
const (
	commandStateInProgress = "InProgress"
	commandStateSuccess    = "Success"
	commandStateFailed     = "Failed"
	commandStateCanceled   = "Canceled"
)

// waitPollInterval is how often a wait re-checks. Commands that matter here
// take tens of seconds, so polling faster buys nothing and costs the agent
// requests it could be spending on the work being waited for.
const waitPollInterval = 3 * time.Second

// defaultCommandWaitTimeout bounds a wait when the caller names no budget. It
// is a ceiling on how long one tool call may hold, not a claim about how long
// commands take: hitting it returns the latest status rather than an error, so
// a caller can decide whether to wait again.
const defaultCommandWaitTimeout = 5 * time.Minute

// maxCommandWaitTimeout caps what a caller may ask for. A tool call that hangs
// for an hour is indistinguishable from a broken one.
const maxCommandWaitTimeout = 15 * time.Minute

// commandStatusView reads just enough of a status payload to find the state.
type commandStatusView struct {
	Commands []struct {
		State string `json:"State"`
	} `json:"Commands"`
}

// commandStateFromStatus reports the state the payload names, or "" when it
// names none.
//
// An unreadable or empty payload deliberately yields "" rather than an error:
// "" is not terminal, so a wait keeps going instead of reporting a command as
// finished on the strength of an answer it could not read.
func commandStateFromStatus(raw json.RawMessage) string {
	var v commandStatusView
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	if len(v.Commands) == 0 {
		return ""
	}
	return v.Commands[0].State
}

// isTerminalCommandState reports whether a wait can stop here. Canceled counts:
// a caller waiting on a command someone else cancelled has to be released.
func isTerminalCommandState(state string) bool {
	switch state {
	case commandStateSuccess, commandStateFailed, commandStateCanceled:
		return true
	default:
		return false
	}
}

// resolveWaitTimeout turns a caller's requested budget into one to honour.
func resolveWaitTimeout(requestedSeconds int) time.Duration {
	if requestedSeconds <= 0 {
		return defaultCommandWaitTimeout
	}
	d := time.Duration(requestedSeconds) * time.Second
	if d > maxCommandWaitTimeout {
		return maxCommandWaitTimeout
	}
	return d
}

// waitForCommand polls until the command reaches an ending, the budget runs
// out, or the caller's context is done. It returns the most recent status
// either way, so a timeout is a report rather than a failure.
func waitForCommand(ctx context.Context, timeout time.Duration,
	fetch func(context.Context) (json.RawMessage, error)) (json.RawMessage, string, error) {
	deadline := time.Now().Add(timeout)
	var last json.RawMessage

	for {
		raw, err := fetch(ctx)
		if err != nil {
			return last, "", err
		}
		last = raw
		state := commandStateFromStatus(raw)
		if isTerminalCommandState(state) {
			return raw, state, nil
		}
		if !time.Now().Add(waitPollInterval).Before(deadline) {
			return last, state, nil
		}
		select {
		case <-ctx.Done():
			return last, state, ctx.Err()
		case <-time.After(waitPollInterval):
		}
	}
}
