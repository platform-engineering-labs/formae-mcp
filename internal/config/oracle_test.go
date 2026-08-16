package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const classicView = `{"schemaVersion":1,"profile":"dev",
 "cli":{"connection":{"mode":"classic","url":"http://localhost","port":49684}}}`

const hostedView = `{"schemaVersion":1,"profile":"prod",
 "cli":{"connection":{"mode":"hosted","endpoint":"https://cloud.formae.ai",
 "installation":"3HzFPXfPDGhwLJJVtaHbmFs6vLa"}}}`

func TestDecodeProfileShow_Classic(t *testing.T) {
	got, err := decodeProfileShow([]byte(classicView))
	if err != nil {
		t.Fatalf("decodeProfileShow: unexpected error: %v", err)
	}
	if got.Profile != "dev" {
		t.Errorf("profile: want %q, got %q", "dev", got.Profile)
	}
	want := Classic{URL: "http://localhost", Port: 49684}
	if got.Conn != Connection(want) {
		t.Errorf("connection: want %#v, got %#v", want, got.Conn)
	}
}

func TestDecodeProfileShow_Hosted(t *testing.T) {
	got, err := decodeProfileShow([]byte(hostedView))
	if err != nil {
		t.Fatalf("decodeProfileShow: unexpected error: %v", err)
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
}

// TestDecodeProfileShow_ToleratesUnknownFields pins that the producer can add
// fields without breaking this consumer.
func TestDecodeProfileShow_ToleratesUnknownFields(t *testing.T) {
	view := `{"schemaVersion":1,"profile":"dev","futureField":{"a":1},
	 "cli":{"connection":{"mode":"classic","url":"http://localhost","port":49684,"extra":true}}}`
	got, err := decodeProfileShow([]byte(view))
	if err != nil {
		t.Fatalf("decodeProfileShow: unexpected error: %v", err)
	}
	if got.Conn != Connection(Classic{URL: "http://localhost", Port: 49684}) {
		t.Errorf("connection: got %#v", got.Conn)
	}
}

func TestDecodeProfileShow_Rejects(t *testing.T) {
	cases := []struct {
		name string
		view string
	}{
		{"unknown schema version", `{"schemaVersion":2,"cli":{"connection":{"mode":"classic","url":"http://x","port":1}}}`},
		{"missing schema version", `{"cli":{"connection":{"mode":"classic","url":"http://x","port":1}}}`},
		{"unknown mode", `{"schemaVersion":1,"cli":{"connection":{"mode":"orbital"}}}`},
		{"missing connection", `{"schemaVersion":1,"profile":"dev","cli":{}}`},
		{"hosted without installation", `{"schemaVersion":1,"cli":{"connection":{"mode":"hosted","endpoint":"https://cloud.formae.ai"}}}`},
		{"hosted with a foreign endpoint", `{"schemaVersion":1,"cli":{"connection":{"mode":"hosted","endpoint":"https://evil.example.com","installation":"3HzFPXfPDGhwLJJVtaHbmFs6vLa"}}}`},
		{"not json", `formae: command not found`},
		{"classic with an empty url", `{"schemaVersion":1,"cli":{"connection":{"mode":"classic","port":49684}}}`},
		{"classic with no port", `{"schemaVersion":1,"cli":{"connection":{"mode":"classic","url":"http://localhost"}}}`},
		{"trailing document", classicView + "\n" + classicView},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeProfileShow([]byte(tc.view)); err == nil {
				t.Fatalf("decodeProfileShow(%s): expected an error, got nil", tc.name)
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

// TestResolveVia_ReadsStdoutIgnoringStderr pins that warnings on stderr are
// never part of the parsed value.
func TestResolveVia_ReadsStdoutIgnoringStderr(t *testing.T) {
	bin := stubFormae(t, "echo 'warning: cli.api is deprecated' >&2\ncat <<'EOF'\n"+classicView+"\nEOF\n")

	got, err := resolveVia(context.Background(), bin, "")
	if err != nil {
		t.Fatalf("resolveVia: unexpected error: %v", err)
	}
	if got.Conn != Connection(Classic{URL: "http://localhost", Port: 49684}) {
		t.Fatalf("connection: got %#v", got.Conn)
	}
}

// TestResolveVia_PassesExactArgv pins the whole command line, so an
// implementation that invents a flag or drops the machine consumer fails here.
// `profile show` takes the name positionally; there is no --profile flag on it
// and none on the root command either, so a flag form would not even parse.
func TestResolveVia_PassesExactArgv(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		want    string
	}{
		{
			"named profile",
			"prod",
			"profile show prod --output-consumer machine --output-schema json",
		},
		{
			"active profile",
			"",
			"profile show --output-consumer machine --output-schema json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := stubFormae(t, "if [ \"$*\" != '"+tc.want+"' ]; then echo \"unexpected argv: $*\" >&2; exit 9; fi\ncat <<'EOF'\n"+classicView+"\nEOF\n")

			if _, err := resolveVia(context.Background(), bin, tc.profile); err != nil {
				t.Fatalf("resolveVia: unexpected error: %v", err)
			}
		})
	}
}

// TestResolveVia_NonZeroExitDoesNotLeakOutput pins that a failure reports the
// exit status rather than the subprocess bytes, which may carry a credential.
func TestResolveVia_NonZeroExitDoesNotLeakOutput(t *testing.T) {
	const secret = "sensitive-subprocess-output"
	bin := stubFormae(t, "echo '"+secret+"'\necho '"+secret+"' >&2\nexit 3\n")

	_, err := resolveVia(context.Background(), bin, "")
	if err == nil {
		t.Fatal("resolveVia: expected an error for a non-zero exit, got nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks subprocess output: %v", err)
	}
	if !strings.Contains(err.Error(), "3") {
		t.Fatalf("error does not name the exit status: %v", err)
	}
}

func TestResolveVia_HonoursCancellation(t *testing.T) {
	bin := stubFormae(t, "sleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := resolveVia(ctx, bin, "")
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
		_, err := resolveVia(context.Background(), bin, "")
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
