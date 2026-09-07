package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// resultText is the text a tool result carries.
func resultText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

// isError reports a tool result the model should read as a failure.
func isError(res *mcp.CallToolResult) bool { return res != nil && res.IsError }

// Signing in is the one thing the MCP cannot do in a single call: the user has
// to be shown a URL and then act on it, so the flow has a middle. `login` returns
// the URL and `complete_login` waits for the outcome, and between them the MCP
// holds the child process — the pending login is in-process memory in the auth
// plugin, holding an open loopback listener, so no second process could resume it.
//
// The child therefore must NOT be tied to the tool call's context. internal/config
// builds its subprocess with exec.CommandContext on the call's context, which is
// right for a read that finishes inside one call and fatal here: it would kill the
// login the moment `login` returned.

// loginStub writes a fake formae that emits the given lines on stdout, one per
// call, with an optional delay before the last of them.
func loginStub(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "formae")
	body := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'formae version: 0.89.0'; exit 0; fi\n" + script
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// serverWithLoginBin returns a server whose formae binary is bin.
func serverWithLoginBin(t *testing.T, bin string) *Server {
	t.Helper()
	s := New("")
	s.ctxResolver = &stubResolver{ec: execctx.Context{FormaeBin: bin}}
	t.Cleanup(s.closePendingLogin)
	return s
}

const startedJSON = `{"schemaVersion":1,"phase":"started","method":"browser","browserUrl":"https://auth.example/authorize?x=1"}`
const completeJSON = `{"schemaVersion":1,"phase":"complete","status":"signed_in","subjectName":"jane",` +
	`"profiles":{"created":["acme-prod"],"updated":[],"renamed":[],"removed":[]},"active":"acme-prod"}`

// login returns the URL without waiting for the flow. Proven by a stub that
// never emits a second document: if the tool waited, this test would hang.
func TestLogin_ReturnsTheURLWithoutWaiting(t *testing.T) {
	bin := loginStub(t, fmt.Sprintf("echo '%s'\nsleep 300\n", startedJSON))
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	text := resultText(res)
	if !strings.Contains(text, "https://auth.example/authorize?x=1") {
		t.Errorf("login did not return the sign-in URL: %s", text)
	}
}

// The regression the review caught. Cancelling the tool call's context — which
// the harness does as soon as the call returns — must not kill the child, or
// complete_login has nothing left to wait on.
func TestLogin_CancellingTheCallDoesNotKillTheChild(t *testing.T) {
	bin := loginStub(t, fmt.Sprintf("echo '%s'\nsleep 0.4\necho '%s'\n", startedJSON, completeJSON))
	s := serverWithLoginBin(t, bin)

	ctx, cancel := context.WithCancel(context.Background())
	if _, _, err := s.handleLogin(ctx, nil, tools.LoginInput{}); err != nil {
		t.Fatalf("login: %v", err)
	}
	cancel() // exactly what the harness does when the call returns.

	res, _, err := s.handleCompleteLogin(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("complete_login: %v", err)
	}
	text := resultText(res)
	if isError(res) {
		t.Fatalf("the sign-in died with its tool call: %s", text)
	}
	if !strings.Contains(text, "acme-prod") {
		t.Errorf("complete_login did not report the profile: %s", text)
	}
}

func TestCompleteLogin_ReportsTheProfilesAndActive(t *testing.T) {
	bin := loginStub(t, fmt.Sprintf("echo '%s'\necho '%s'\n", startedJSON, completeJSON))
	s := serverWithLoginBin(t, bin)

	if _, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{}); err != nil {
		t.Fatalf("login: %v", err)
	}
	res, _, err := s.handleCompleteLogin(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("complete_login: %v", err)
	}
	text := resultText(res)
	for _, want := range []string{"acme-prod", "jane"} {
		if !strings.Contains(text, want) {
			t.Errorf("complete_login output does not mention %q: %s", want, text)
		}
	}
}

// A session already open emits only the completion document. There is nothing
// for the user to do, so login reports the outcome directly and leaves no child.
func TestLogin_AnOpenSessionCompletesImmediately(t *testing.T) {
	already := `{"schemaVersion":1,"phase":"complete","status":"already_authenticated","subjectName":"jane",` +
		`"profiles":{"created":[],"updated":[],"renamed":[],"removed":[]},"active":"acme-prod"}`
	bin := loginStub(t, fmt.Sprintf("echo '%s'\n", already))
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if isError(res) {
		t.Fatalf("an open session was reported as a failure: %s", resultText(res))
	}
	if !strings.Contains(strings.ToLower(resultText(res)), "already signed in") {
		t.Errorf("an open session was not reported as such: %s", resultText(res))
	}
}

func TestCompleteLogin_WithNoLoginSaysSo(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, "exit 0\n"))

	res, _, err := s.handleCompleteLogin(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("complete_login: %v", err)
	}
	if !isError(res) {
		t.Fatal("complete_login without a login reported success")
	}
	if !strings.Contains(resultText(res), "login") {
		t.Errorf("the message does not point at the login tool: %s", resultText(res))
	}
}

// A second login replaces the first, and the first child is gone — the whole
// group, not just the leader, so a plugin subprocess cannot outlive it holding
// the loopback port the next flow needs.
func TestLogin_ASecondLoginReplacesTheFirst(t *testing.T) {
	bin := loginStub(t, fmt.Sprintf("echo '%s'\nsleep 300\n", startedJSON))
	s := serverWithLoginBin(t, bin)

	if _, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{}); err != nil {
		t.Fatalf("first login: %v", err)
	}
	first := s.pendingPID()
	if first == 0 {
		t.Fatal("no child was recorded")
	}

	if _, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{}); err != nil {
		t.Fatalf("second login: %v", err)
	}
	if s.pendingPID() == first {
		t.Fatal("the second login reused the first child")
	}

	// The process group is gone. Signal 0 probes without delivering anything.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-first, 0); err != nil {
			return // reaped.
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the replaced login's process group is still alive")
}

// A failure envelope is surfaced by its declared code, and the producer's own
// message never is: it is built from an auth plugin's error string, and a plugin
// is not a trusted source of prose.
func TestLogin_AFailureEnvelopeSurfacesItsCode(t *testing.T) {
	env := `{"schemaVersion":1,"code":"auth_failed","message":"SECRET-PROSE-DO-NOT-SHOW",` +
		`"details":{"pluginCode":"session_expired"}}`
	bin := loginStub(t, fmt.Sprintf("echo '%s'\nexit 1\n", env))
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !isError(res) {
		t.Fatal("a refused sign-in reported success")
	}
	text := resultText(res)
	if strings.Contains(text, "SECRET-PROSE-DO-NOT-SHOW") {
		t.Errorf("the producer's free text reached the tool result: %s", text)
	}
	if !strings.Contains(text, "session_expired") && !strings.Contains(text, "expired") {
		t.Errorf("the declared reason was not surfaced: %s", text)
	}
}

// A non-zero exit whose stdout is not an envelope reports the status and not the
// bytes. This is a supported path: argv the command cannot parse fails before the
// flags that say how to render a failure are established.
func TestLogin_UnparseableFailureReportsTheStatusNotTheBytes(t *testing.T) {
	bin := loginStub(t, "echo 'unknown flag: --hosted'\nexit 2\n")
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !isError(res) {
		t.Fatal("a failed sign-in reported success")
	}
	if strings.Contains(resultText(res), "unknown flag") {
		t.Errorf("subprocess output reached the tool result: %s", resultText(res))
	}
}

// The argv is pinned whole, both forms. A consumer built against a sketch of a
// producer is what shipped this repo's argv defect once already.
func TestLogin_PinsTheArgv(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input tools.LoginInput
		want  string
	}{
		{
			name:  "browser",
			input: tools.LoginInput{},
			want:  "login --hosted --output-consumer machine --output-schema json",
		},
		{
			name:  "device",
			input: tools.LoginInput{Device: true},
			want:  "login --hosted --device --output-consumer machine --output-schema json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argsFile := filepath.Join(t.TempDir(), "args")
			bin := loginStub(t, fmt.Sprintf("echo \"$@\" > %s\necho '%s'\nsleep 300\n", argsFile, startedJSON))
			s := serverWithLoginBin(t, bin)

			if _, _, err := s.handleLogin(context.Background(), nil, tc.input); err != nil {
				t.Fatalf("login: %v", err)
			}

			recorded, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("the stub recorded no argv: %v", err)
			}
			if got := strings.TrimSpace(string(recorded)); got != tc.want {
				t.Errorf("argv = %q, want %q", got, tc.want)
			}
		})
	}
}

// Neither tool is gated by the onboarding check, and it falls out of them not
// being agent-backed rather than being carved out: signing in is exactly what a
// machine with no profile has to be able to do.
func TestLogin_IsNotStoppedByTheOnboardingGate(t *testing.T) {
	t.Setenv("FORMAE_CONFIG_DIR", t.TempDir()) // an empty store.
	bin := loginStub(t, fmt.Sprintf("echo '%s'\nsleep 300\n", startedJSON))
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if isError(res) {
		t.Fatalf("login was refused on the machine it exists for: %s", resultText(res))
	}
}

// The device flow's code and expiry reach the caller, since that is the whole
// content of the instruction on a machine with no browser.
func TestLogin_DeviceFlowReturnsTheCode(t *testing.T) {
	dev := `{"schemaVersion":1,"phase":"started","method":"device",` +
		`"verificationUri":"https://auth.example/device","userCode":"WDJB-MJHT","expiresInSeconds":900}`
	bin := loginStub(t, fmt.Sprintf("echo '%s'\nsleep 300\n", dev))
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleLogin(context.Background(), nil, tools.LoginInput{Device: true})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	text := resultText(res)
	for _, want := range []string{"https://auth.example/device", "WDJB-MJHT"} {
		if !strings.Contains(text, want) {
			t.Errorf("the device instruction does not carry %q: %s", want, text)
		}
	}
}
