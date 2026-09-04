package config

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The resolve view is flat: the connection sits at the top level, not under a
// `cli` key the way `profile show` nests it.
const classicView = `{"schemaVersion":1,"profile":"dev",
 "connection":{"mode":"classic","url":"http://localhost","port":49684}}`

const hostedView = `{"schemaVersion":1,"profile":"prod",
 "connection":{"mode":"hosted","endpoint":"https://cloud.formae.ai",
 "installation":"3HzFPXfPDGhwLJJVtaHbmFs6vLa","auth":{"type":"oidc"}},
 "credential":"Bearer live-token"}`

// token is recognisable, so a leak into an error shows up in a substring check.
const token = "Bearer sup3rs3cr3t"

func TestDecodeResolved_Classic(t *testing.T) {
	got, err := decodeResolved([]byte(classicView))
	if err != nil {
		t.Fatalf("decodeResolved: unexpected error: %v", err)
	}
	if got.Profile != "dev" {
		t.Errorf("profile: want %q, got %q", "dev", got.Profile)
	}
	want := Classic{URL: "http://localhost", Port: 49684}
	if got.Conn != Connection(want) {
		t.Errorf("connection: want %#v, got %#v", want, got.Conn)
	}
	if !got.Credential.IsZero() {
		t.Error("a classic connection must carry no credential")
	}
}

func TestDecodeResolved_Hosted(t *testing.T) {
	got, err := decodeResolved([]byte(hostedView))
	if err != nil {
		t.Fatalf("decodeResolved: unexpected error: %v", err)
	}
	if got.Profile != "prod" {
		t.Errorf("profile: want %q, got %q", "prod", got.Profile)
	}
	want := Hosted{
		Endpoint:     "https://cloud.formae.ai",
		Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa",
	}
	if got.Conn != Connection(want) {
		t.Errorf("connection: want %#v, got %#v", want, got.Conn)
	}
	if got.Credential.Reveal() != "Bearer live-token" {
		t.Errorf("credential: got %q", got.Credential.Reveal())
	}
}

// The producer can add fields without breaking this consumer. The hosted arm's
// `auth` object is one such field today: it is reduced to a type discriminator
// and this consumer has no use for it.
func TestDecodeResolved_ToleratesUnknownFields(t *testing.T) {
	view := `{"schemaVersion":1,"profile":"dev","futureField":{"a":1},
	 "connection":{"mode":"classic","url":"http://localhost","port":49684,"extra":true}}`
	got, err := decodeResolved([]byte(view))
	if err != nil {
		t.Fatalf("decodeResolved: unexpected error: %v", err)
	}
	if got.Conn != Connection(Classic{URL: "http://localhost", Port: 49684}) {
		t.Errorf("connection: got %#v", got.Conn)
	}
}

func TestDecodeResolved_Rejects(t *testing.T) {
	cases := []struct {
		name string
		view string
	}{
		{"unknown schema version", `{"schemaVersion":2,"connection":{"mode":"classic","url":"http://x","port":1}}`},
		{"missing schema version", `{"connection":{"mode":"classic","url":"http://x","port":1}}`},
		{"unknown mode", `{"schemaVersion":1,"connection":{"mode":"orbital"}}`},
		{"missing connection", `{"schemaVersion":1,"profile":"dev"}`},
		{"hosted without installation", `{"schemaVersion":1,"connection":{"mode":"hosted","endpoint":"https://cloud.formae.ai"},"credential":"Bearer x"}`},
		{"hosted with a foreign endpoint", `{"schemaVersion":1,"connection":{"mode":"hosted","endpoint":"https://evil.example.com","installation":"3HzFPXfPDGhwLJJVtaHbmFs6vLa"},"credential":"Bearer x"}`},
		// A hosted connection that cannot be authenticated is not a usable
		// connection. The producer refuses to emit one; this is the consumer
		// refusing to invent one.
		{"hosted with no credential", `{"schemaVersion":1,"connection":{"mode":"hosted","endpoint":"https://cloud.formae.ai","installation":"3HzFPXfPDGhwLJJVtaHbmFs6vLa"}}`},
		// The MCP sends a self-hosted agent no credential. Accepting one here
		// would make that non-goal reachable by a producer bug.
		{"classic with a credential", `{"schemaVersion":1,"connection":{"mode":"classic","url":"http://localhost","port":49684},"credential":"Bearer x"}`},
		{"not json", `formae: command not found`},
		{"classic with an empty url", `{"schemaVersion":1,"connection":{"mode":"classic","port":49684}}`},
		{"classic with no port", `{"schemaVersion":1,"connection":{"mode":"classic","url":"http://localhost"}}`},
		{"trailing document", classicView + "\n" + classicView},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeResolved([]byte(tc.view)); err == nil {
				t.Fatalf("decodeResolved(%s): expected an error, got nil", tc.name)
			}
		})
	}
}

func envelope(t *testing.T, code, message string, details map[string]any) string {
	t.Helper()
	e := map[string]any{"schemaVersion": 1, "code": code, "message": message}
	if details != nil {
		e["details"] = details
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("building an envelope: %v", err)
	}
	return string(b)
}

func TestDecodeFailure_AmbiguousProfileCarriesTheCandidates(t *testing.T) {
	env := envelope(t, "ambiguous_profile", "more than one profile exists", map[string]any{
		"candidates": []string{"prod", "staging"},
		"active":     "prod",
	})

	err := decodeFailure([]byte(env), 1)

	var amb *AmbiguousProfileError
	if !errors.As(err, &amb) {
		t.Fatalf("want an AmbiguousProfileError, got %T: %v", err, err)
	}
	if strings.Join(amb.Candidates, ",") != "prod,staging" {
		t.Errorf("candidates: got %v", amb.Candidates)
	}
	if amb.Active != "prod" {
		t.Errorf("active: got %q", amb.Active)
	}
	for _, want := range []string{"prod", "staging", "profile"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message must name %q so the caller can retry: %v", want, err)
		}
	}
}

func TestDecodeFailure_DeclaredCodesCarryTheirCode(t *testing.T) {
	for _, code := range []string{"auth_failed", "untrusted_issuer", "no_connection", "internal"} {
		t.Run(code, func(t *testing.T) {
			err := decodeFailure([]byte(envelope(t, code, "some producer prose", nil)), 1)

			var re *ResolveError
			if !errors.As(err, &re) {
				t.Fatalf("want a ResolveError, got %T: %v", err, err)
			}
			if re.Code != code {
				t.Errorf("code: want %q, got %q", code, re.Code)
			}
		})
	}
}

// The producer builds these messages out of an auth plugin's error string or an
// arbitrary err.Error(). A Pkl evaluation failure quotes source lines, and a
// classic profile can hold an inline password, so the free text is closed here.
func TestDecodeFailure_NeverSurfacesTheProducersMessage(t *testing.T) {
	env := envelope(t, "internal", "cannot read profile: password = \""+token+"\"", nil)

	err := decodeFailure([]byte(env), 1)

	if strings.Contains(err.Error(), token) {
		t.Fatalf("the producer's message reached the consumer: %v", err)
	}
	if strings.Contains(err.Error(), "password") {
		t.Fatalf("the producer's message reached the consumer: %v", err)
	}
	if !strings.Contains(err.Error(), "formae connection resolve") {
		t.Fatalf("the fixed text must point somewhere the reader can look: %v", err)
	}
}

// pkg/auth.ErrorCode is a bare string alias and the producer copies whatever the
// plugin returned into details.pluginCode without checking it, so this is an
// open channel wearing a closed channel's name.
func TestDecodeFailure_ValidatesThePluginCode(t *testing.T) {
	t.Run("a declared code is kept", func(t *testing.T) {
		env := envelope(t, "auth_failed", "session is gone", map[string]any{"pluginCode": "session_expired"})

		err := decodeFailure([]byte(env), 1)

		var re *ResolveError
		if !errors.As(err, &re) {
			t.Fatalf("want a ResolveError, got %T", err)
		}
		if re.PluginCode != "session_expired" {
			t.Errorf("pluginCode: got %q", re.PluginCode)
		}
		if !strings.Contains(err.Error(), "session_expired") {
			t.Errorf("a declared plugin code carries diagnostic value and should be shown: %v", err)
		}
	})

	t.Run("anything else is opaque", func(t *testing.T) {
		env := envelope(t, "auth_failed", "refused", map[string]any{"pluginCode": token})

		err := decodeFailure([]byte(env), 1)

		if strings.Contains(err.Error(), token) {
			t.Fatalf("an unvalidated plugin code reached the consumer: %v", err)
		}
		var re *ResolveError
		if !errors.As(err, &re) {
			t.Fatalf("want a ResolveError, got %T", err)
		}
		if re.PluginCode == token {
			t.Fatalf("an unvalidated plugin code was kept: %q", re.PluginCode)
		}
	})
}

func TestDecodeFailure_Rejects(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unknown envelope schema version", `{"schemaVersion":2,"code":"internal","message":"x"}`},
		{"missing envelope schema version", `{"code":"internal","message":"x"}`},
		{"an unregistered code", envelope(t, "teapot", "x", nil)},
		// Argv the command cannot parse fails before the output flags are
		// established, so it exits non-zero with no envelope at all. The
		// producer pins that, so this is a supported path, not a defensive one.
		{"no envelope at all", "Error: unknown flag: --nope"},
		{"empty output", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeFailure([]byte(tc.body), 7)
			if err == nil {
				t.Fatal("expected an error")
			}
			var amb *AmbiguousProfileError
			if errors.As(err, &amb) {
				t.Fatal("an unreadable envelope must not decode as ambiguity")
			}
			if !strings.Contains(err.Error(), "7") {
				t.Errorf("an unreadable failure must name the exit status: %v", err)
			}
			if strings.Contains(err.Error(), "unknown flag") {
				t.Errorf("an unreadable failure must not echo the bytes: %v", err)
			}
		})
	}
}

// stubFormae writes an executable stand-in for the formae CLI. It answers
// --version in the real format, because callers version-gate before resolving.
func stubFormae(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "formae")
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 'formae version: 0.89.0'; exit 0; fi\n" +
		script
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}
	return path
}

// TestResolveVia_PassesExactArgv pins the whole command line. --profile is a
// local flag on `resolve`, so it follows the subcommand — the opposite of
// `profile show`, where the name is positional. An implementation that carries
// the old shape over passes against a lenient stub and fails against the real
// binary, which is how slice 1's argv defect got in.
func TestResolveVia_PassesExactArgv(t *testing.T) {
	cases := []struct {
		name         string
		profile      string
		forceRefresh bool
		want         string
	}{
		{
			"active profile",
			"", false,
			"connection resolve --output-consumer machine --output-schema json",
		},
		{
			"named profile",
			"prod", false,
			"connection resolve --profile prod --output-consumer machine --output-schema json",
		},
		{
			"named profile with a forced refresh",
			"prod", true,
			"connection resolve --profile prod --output-consumer machine --output-schema json --force-refresh",
		},
		{
			"active profile with a forced refresh",
			"", true,
			"connection resolve --output-consumer machine --output-schema json --force-refresh",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := stubFormae(t, "if [ \"$*\" != '"+tc.want+"' ]; then echo \"unexpected argv: $*\" >&2; exit 9; fi\ncat <<'EOF'\n"+classicView+"\nEOF\n")

			if _, err := resolveVia(context.Background(), bin, tc.profile, tc.forceRefresh); err != nil {
				t.Fatalf("resolveVia: unexpected error: %v", err)
			}
		})
	}
}

// TestResolveVia_ReadsStdoutIgnoringStderr pins that warnings on stderr are
// never part of the parsed value.
func TestResolveVia_ReadsStdoutIgnoringStderr(t *testing.T) {
	bin := stubFormae(t, "echo 'warning: cli.api is deprecated' >&2\ncat <<'EOF'\n"+classicView+"\nEOF\n")

	got, err := resolveVia(context.Background(), bin, "", false)
	if err != nil {
		t.Fatalf("resolveVia: unexpected error: %v", err)
	}
	if got.Conn != Connection(Classic{URL: "http://localhost", Port: 49684}) {
		t.Fatalf("connection: got %#v", got.Conn)
	}
}

// A declared failure reaches the caller as its typed error, which means reading
// stdout on a non-zero exit rather than reporting the status and stopping.
func TestResolveVia_DecodesADeclaredFailure(t *testing.T) {
	env := envelope(t, "ambiguous_profile", "pick one", map[string]any{
		"candidates": []string{"prod", "staging"},
		"active":     "prod",
	})
	bin := stubFormae(t, "cat <<'EOF'\n"+env+"\nEOF\nexit 1\n")

	_, err := resolveVia(context.Background(), bin, "", false)

	var amb *AmbiguousProfileError
	if !errors.As(err, &amb) {
		t.Fatalf("want an AmbiguousProfileError, got %T: %v", err, err)
	}
}

// TestResolveVia_NonZeroExitDoesNotLeakOutput pins that a failure the consumer
// cannot read reports the exit status rather than the subprocess bytes, which
// may carry a credential or a profile's contents.
func TestResolveVia_NonZeroExitDoesNotLeakOutput(t *testing.T) {
	const leaked = "sensitive-subprocess-output"
	bin := stubFormae(t, "echo '"+leaked+"'\necho '"+leaked+"' >&2\nexit 3\n")

	_, err := resolveVia(context.Background(), bin, "", false)
	if err == nil {
		t.Fatal("resolveVia: expected an error for a non-zero exit, got nil")
	}
	if strings.Contains(err.Error(), leaked) {
		t.Fatalf("error leaks subprocess output: %v", err)
	}
	if !strings.Contains(err.Error(), "3") {
		t.Fatalf("error does not name the exit status: %v", err)
	}
}

// A credential must not reach an error on any path, nor a rendering of the
// value the resolver returns.
func TestResolveVia_ContainsTheCredential(t *testing.T) {
	t.Run("a rendering of the resolved value", func(t *testing.T) {
		bin := stubFormae(t, "cat <<'EOF'\n"+hostedView+"\nEOF\n")

		got, err := resolveVia(context.Background(), bin, "", false)
		if err != nil {
			t.Fatalf("resolveVia: %v", err)
		}
		out, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshalling Resolved: %v", err)
		}
		if strings.Contains(string(out), "live-token") {
			t.Fatalf("a JSON rendering of Resolved leaked the credential: %s", out)
		}
	})

	t.Run("a success document that then fails to validate", func(t *testing.T) {
		bad := `{"schemaVersion":1,"profile":"prod","connection":{"mode":"hosted",` +
			`"endpoint":"https://evil.example.com","installation":"3HzFPXfPDGhwLJJVtaHbmFs6vLa"},` +
			`"credential":"` + token + `"}`
		bin := stubFormae(t, "cat <<'EOF'\n"+bad+"\nEOF\n")

		_, err := resolveVia(context.Background(), bin, "", false)
		if err == nil {
			t.Fatal("expected a foreign endpoint to be rejected")
		}
		if strings.Contains(err.Error(), token) {
			t.Fatalf("the rejection leaked the credential: %v", err)
		}
	})
}

func TestResolveVia_HonoursCancellation(t *testing.T) {
	bin := stubFormae(t, "sleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := resolveVia(ctx, bin, "", false)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("resolveVia with a cancelled context: expected an error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resolveVia did not return on a cancelled context")
	}
}

// TestResolveVia_BoundsOutputSizePromptly pins that hitting the output bound
// terminates the child instead of waiting for it to finish writing.
func TestResolveVia_BoundsOutputSizePromptly(t *testing.T) {
	bin := stubFormae(t, "exec yes\n")

	done := make(chan error, 1)
	go func() {
		_, err := resolveVia(context.Background(), bin, "", false)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, errOutputTooLarge) {
			t.Fatalf("want errOutputTooLarge, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resolveVia did not bound unbounded output promptly")
	}
}
