package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// A user who reaches for any agent-backed tool on a machine with nothing
// configured has to be asked what they meant, because their intent — hosted or
// self-hosted — is unguessable and resolving a connection decides it for them by
// creating a classic localhost default.
//
// The property under test throughout is that resolution is *not reached*. Not
// that the output looks a certain way: the act of looking is what destroys the
// answer, so a gate that returned the right message after resolving would have
// failed at the only thing it exists for.

// gatedServer returns a server whose resolver counts calls, pointed at dir.
func gatedServer(t *testing.T, dir string) (*Server, *stubResolver) {
	t.Helper()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	s := New("")
	r := &stubResolver{ec: execctx.Context{ProfileName: "default"}}
	s.ctxResolver = r
	return s, r
}

// configured writes a profile and points the active pointer at it.
func configured(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", "default.pkl"),
		[]byte("amends \"formae:/Config.pkl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("default\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGate_AnEmptyStoreNeverReachesResolution(t *testing.T) {
	s, r := gatedServer(t, t.TempDir())

	_, err := s.resolveCtx(context.Background(), "")

	if err == nil {
		t.Fatal("an unconfigured machine resolved a connection")
	}
	if r.calls != 0 {
		t.Errorf("resolution was reached %d times; looking is what destroys the answer", r.calls)
	}
}

// The instruction is the whole value of the gate: a model reads it and acts,
// so it has to name both branches and the exact call that starts each.
func TestGate_TheInstructionNamesBothBranches(t *testing.T) {
	s, _ := gatedServer(t, t.TempDir())

	_, err := s.resolveCtx(context.Background(), "")
	if err == nil {
		t.Fatal("expected the onboarding instruction")
	}
	msg := err.Error()

	for _, want := range []string{
		"use_profile",         // the self-hosted action
		`"default"`,           // with this name
		"console.formae.ai",   // where a hosted user creates an account
		"/formae:setup",       // what they run when they come back
		"formae profile edit", // the self-hoster's next step
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the onboarding instruction does not mention %q:\n%s", want, msg)
		}
	}

	// create_profile --force migrates a legacy config and then deletes it. It
	// must not be what the MCP tells anyone to run.
	if strings.Contains(msg, "force") {
		t.Errorf("the instruction offers a forced create, which can destroy a legacy config:\n%s", msg)
	}
}

// A configured machine is untouched by any of this.
func TestGate_AConfiguredStoreResolvesAsBefore(t *testing.T) {
	dir := t.TempDir()
	configured(t, dir)
	s, r := gatedServer(t, dir)

	if _, err := s.resolveCtx(context.Background(), ""); err != nil {
		t.Fatalf("a configured machine was refused: %v", err)
	}
	if r.calls != 1 {
		t.Errorf("resolution ran %d times, want 1", r.calls)
	}
}

// The finding an adversarial review caught, as a test. A user whose whole
// configuration is a legacy formae.conf.pkl has no profiles, so a gate asking
// "are there profiles" fires for them — and the remedy it offers migrates that
// file and, with force, deletes it.
func TestGate_ALegacyConfigIsNotOnboarded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "formae.conf.pkl"),
		[]byte("amends \"formae:/Config.pkl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, r := gatedServer(t, dir)

	if _, err := s.resolveCtx(context.Background(), ""); err != nil {
		t.Fatalf("a user with a legacy config was sent through onboarding: %v", err)
	}
	if r.calls != 1 {
		t.Errorf("resolution ran %d times, want 1", r.calls)
	}
}

// Profiles with no usable pointer: the CLI has a good message for this and
// resolution would fail, so the MCP names the profiles rather than letting it
// become a generic connection error.
func TestGate_ProfilesWithNoPointerAskForAChoice(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"prod", "staging"} {
		if err := os.WriteFile(filepath.Join(dir, "profiles", n+".pkl"),
			[]byte("amends \"formae:/Config.pkl\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, r := gatedServer(t, dir)

	_, err := s.resolveCtx(context.Background(), "")
	if err == nil {
		t.Fatal("a store with no active profile resolved")
	}
	if r.calls != 0 {
		t.Errorf("resolution was reached %d times", r.calls)
	}
	for _, want := range []string{"use_profile", "prod", "staging"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the instruction does not mention %q:\n%s", want, err.Error())
		}
	}
}

// forcedEndpoint is the seam a test injects a mock agent URL through, and it has
// to keep working with no profile store at all. Ordered explicitly because
// getting it the other way round breaks every existing server test for a reason
// that would look like the gate working.
func TestGate_AForcedEndpointIsNotGated(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir())
	s := New("http://127.0.0.1:1")
	r := &stubResolver{ec: execctx.Context{}}
	s.ctxResolver = r

	ec, err := s.resolveCtx(context.Background(), "")
	if err != nil {
		t.Fatalf("a forced endpoint was gated: %v", err)
	}
	if _, ok := ec.Conn.(config.Classic); !ok {
		t.Errorf("the forced endpoint did not produce a classic connection: %#v", ec.Conn)
	}
}

// A hosted profile whose session has lapsed is told to sign in. The signal
// already exists — resolution reports auth_failed with the plugin's own code —
// so this is a rendering of an answer rather than a second probe that could
// disagree with the first.
func TestGate_ALapsedHostedSessionIsToldToSignIn(t *testing.T) {
	for _, code := range []string{"not_logged_in", "session_expired"} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			configured(t, dir)
			s, r := gatedServer(t, dir)
			r.err = &config.ResolveError{Code: "auth_failed", PluginCode: code}

			_, err := s.resolveCtx(context.Background(), "")
			if err == nil {
				t.Fatal("a lapsed session resolved")
			}
			if !strings.Contains(err.Error(), "login") {
				t.Errorf("a lapsed session was not told to sign in:\n%s", err.Error())
			}
		})
	}
}

// Signing in does not fix an unreachable issuer or a plugin that does not support
// the operation, so sending a user through a browser flow that cannot help is
// worse than saying what happened.
func TestGate_OtherAuthFailuresAreNotToldToSignIn(t *testing.T) {
	for _, code := range []string{"issuer_unreachable", "unsupported", "unrecognised", ""} {
		t.Run("code="+code, func(t *testing.T) {
			dir := t.TempDir()
			configured(t, dir)
			s, r := gatedServer(t, dir)
			r.err = &config.ResolveError{Code: "auth_failed", PluginCode: code}

			_, err := s.resolveCtx(context.Background(), "")
			if err == nil {
				t.Fatal("expected the resolution failure to be reported")
			}
			if strings.Contains(err.Error(), "call the login tool") {
				t.Errorf("%q was offered a sign-in that cannot fix it:\n%s", code, err.Error())
			}
		})
	}
}

// A classic profile is never offered a sign-in on any path: there is nothing to
// log in to, and the refusal it gets today is correct.
func TestGate_AClassicProfileIsNeverOfferedASignIn(t *testing.T) {
	dir := t.TempDir()
	configured(t, dir)
	s, r := gatedServer(t, dir)
	r.err = &config.ResolveError{Code: "no_connection"}

	_, err := s.resolveCtx(context.Background(), "")
	if err == nil {
		t.Fatal("expected the resolution failure")
	}
	if strings.Contains(err.Error(), "login") {
		t.Errorf("a classic profile was offered a sign-in:\n%s", err.Error())
	}
}

// Ambiguity is untouched: it already carries its own instruction.
func TestGate_AmbiguityIsPassedThroughUnchanged(t *testing.T) {
	dir := t.TempDir()
	configured(t, dir)
	s, r := gatedServer(t, dir)
	r.err = &config.AmbiguousProfileError{Candidates: []string{"a", "b"}, Active: "a"}

	_, err := s.resolveCtx(context.Background(), "")
	if err == nil {
		t.Fatal("expected the ambiguity error")
	}
	var ambiguous *config.AmbiguousProfileError
	if !errors.As(err, &ambiguous) {
		t.Errorf("ambiguity did not survive the gate: %T", err)
	}
}

// The gate sits in resolveCtx, which every agent-backed handler passes through,
// so no handler has to opt in. This walks a representative set and fails if any
// of them reaches resolution on an unconfigured machine.
func TestGate_NoAgentBackedHandlerEscapesTheGate(t *testing.T) {
	handlers := map[string]func(*Server) error{
		"list_stacks": func(s *Server) error {
			_, _, err := s.handleListStacks(context.Background(), nil, tools.ProfileInput{})
			return err
		},
		"list_resources": func(s *Server) error {
			_, _, err := s.handleListResources(context.Background(), nil, tools.ListResourcesInput{})
			return err
		},
		"check_health": func(s *Server) error {
			_, _, err := s.handleCheckHealth(context.Background(), nil, tools.ProfileInput{})
			return err
		},
		"list_targets": func(s *Server) error {
			_, _, err := s.handleListTargets(context.Background(), nil, tools.ListTargetsInput{})
			return err
		},
		"get_agent_stats": func(s *Server) error {
			_, _, err := s.handleGetAgentStats(context.Background(), nil, tools.ProfileInput{})
			return err
		},
	}

	for name, call := range handlers {
		t.Run(name, func(t *testing.T) {
			s, r := gatedServer(t, t.TempDir())
			if err := call(s); err != nil {
				t.Fatalf("handler returned a transport error: %v", err)
			}
			if r.calls != 0 {
				t.Errorf("%s reached resolution on an unconfigured machine", name)
			}
		})
	}
}
