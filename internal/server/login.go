package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// Signing in is the one thing the MCP cannot do in a single call. The user has to
// be shown a URL and then act on it, so the flow has a middle: `login` returns the
// URL, `complete_login` waits for the outcome, and between them the MCP holds the
// child.
//
// It holds the child rather than starting a second one because the pending login
// is in-process memory in the auth plugin — a single slot holding, for the browser
// flow, an open loopback listener — so no second process could resume a session
// the first one began.
//
// The child is started from a context the *server* owns, and that is the whole
// design rather than a detail. internal/config builds its subprocess with
// exec.CommandContext on the tool call's context, which is right for a read that
// finishes inside one call; here it would kill the login the moment `login`
// returned and leave `complete_login` nothing to wait on. What is copied from
// that code is the process group and the stderr drain, which are right for any
// child.
//
// One pending login at a time, not because the layer below enforces it — the
// plugin's slot is per plugin process and the CLI starts one per invocation — but
// because there is one user and one conversation, and a second `login` means the
// first was abandoned.

// loginMaxDocument bounds a single document read from the child.
//
// bufio.Scanner rather than ReadBytes, because ReadBytes accumulates a whole
// line before anything can reject it: a length check after the fact is a check
// made once the memory has already been spent. Scanner refuses past the limit
// instead of buying it first.
const loginMaxDocument = 1 << 20 // 1 MiB

// pendingLogin is a sign-in waiting for the user to finish it.
type pendingLogin struct {
	cmd *exec.Cmd
	// out is shared across the two tool calls, so the second reads where the
	// first stopped rather than from a fresh view of the pipe.
	out    *bufio.Scanner
	cancel context.CancelFunc
}

// startedDoc is what `login` reports: what the user has to do next.
type startedDoc struct {
	SchemaVersion int    `json:"schemaVersion"`
	Phase         string `json:"phase"`
	Method        string `json:"method"`

	BrowserURL string `json:"browserUrl"`

	VerificationURI  string `json:"verificationUri"`
	UserCode         string `json:"userCode"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

// completeDoc is what `complete_login` reports: who signed in and what it wrote.
type completeDoc struct {
	SchemaVersion int    `json:"schemaVersion"`
	Phase         string `json:"phase"`
	Status        string `json:"status"`
	Subject       string `json:"subject"`
	SubjectName   string `json:"subjectName"`
	Profiles      struct {
		Created []string `json:"created"`
		Updated []string `json:"updated"`
		Renamed []string `json:"renamed"`
		Removed []string `json:"removed"`
	} `json:"profiles"`
	Active   string   `json:"active"`
	Warnings []string `json:"warnings"`
}

// loginSchemaVersion is the document shape this build understands. Checked before
// any other field, so a document from a newer formae is an error rather than a
// guess.
const loginSchemaVersion = 1

// handleLogin starts a hosted sign-in and returns what the user has to do next.
//
// It does not wait for the flow. A session already open produces the completion
// document directly, which is what makes driving setup twice harmless.
func (s *Server) handleLogin(ctx context.Context, _ *mcp.CallToolRequest, input tools.LoginInput) (*mcp.CallToolResult, any, error) {
	args := []string{"login", "--hosted"}
	if input.Device {
		args = append(args, "--device")
	}
	args = append(args, "--output-consumer", "machine", "--output-schema", "json")

	// Deliberately not ctx: this child outlives the call that starts it. The
	// server's context is what ends it.
	childCtx, cancel := context.WithCancel(s.lifetime())

	cmd := exec.CommandContext(childCtx, s.ctxResolver.Bin(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { killGroup(cmd); return nil }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return errorResult(fmt.Errorf("start a hosted sign-in: %w", err)), nil, nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return errorResult(fmt.Errorf("start a hosted sign-in: %w", err)), nil, nil
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return errorResult(fmt.Errorf("run %s: %w", s.ctxResolver.Bin(), err)), nil, nil
	}
	// Drained and discarded so the child cannot block on a full pipe.
	go func() { _, _ = io.Copy(io.Discard, stderrPipe) }()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), loginMaxDocument)
	p := &pendingLogin{cmd: cmd, out: scanner, cancel: cancel}
	s.setPendingLogin(p)

	line, err := readDocument(p.out)
	if err != nil {
		return errorResult(s.failLogin(p, err)), nil, nil
	}

	// Three things can arrive first, and which one decides everything after.
	// A failure before the flow opens — a missing plugin, an unreachable issuer —
	// emits an envelope and no started document at all, so classifying on "is it
	// a started document" would read that envelope as a sign-in with no URL in it.
	switch classify(line) {
	case docFailure:
		return errorResult(s.failLogin(p, envelopeError(line))), nil, nil

	case docComplete:
		// A session already open: nothing for the user to do, and no child left.
		done, derr := decodeComplete(line)
		if derr != nil {
			return errorResult(s.failLogin(p, derr)), nil, nil
		}
		s.closePendingLogin()
		return textResult(renderComplete(done)), nil, nil

	case docStarted:
		var started startedDoc
		if err := json.Unmarshal(line, &started); err != nil {
			return errorResult(s.failLogin(p, errUnreadableLogin)), nil, nil
		}
		return textResult(renderStarted(started)), nil, nil

	default:
		return errorResult(s.failLogin(p, errUnreadableLogin)), nil, nil
	}
}

// handleCompleteLogin waits for the sign-in `login` began and reports what it did.
func (s *Server) handleCompleteLogin(_ context.Context, _ *mcp.CallToolRequest, _ tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	p := s.takePendingLogin()
	if p == nil {
		return errorResult(errors.New(
			"no sign-in is in progress; call the login tool first, show the user the URL it returns, " +
				"and call this once they have finished in the browser")), nil, nil
	}
	defer p.cancel()

	line, err := readDocument(p.out)
	if err != nil {
		return errorResult(p.explain(err)), nil, nil
	}
	done, derr := decodeComplete(line)
	if derr != nil {
		return errorResult(p.explain(derr)), nil, nil
	}
	_ = p.cmd.Wait()
	return textResult(renderComplete(done)), nil, nil
}

// lifetime is the context the login child is tied to. It is the server's, so a
// sign-in survives the tool call that began it and ends when the server does.
func (s *Server) lifetime() context.Context {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.runCtx == nil {
		return context.Background()
	}
	return s.runCtx
}

// setPendingLogin installs p, ending whatever sign-in was pending. A second
// login means the first was abandoned, and leaving it running would hold the
// loopback port the next browser flow needs.
func (s *Server) setPendingLogin(p *pendingLogin) {
	s.loginMu.Lock()
	prev := s.pending
	s.pending = p
	s.loginMu.Unlock()

	if prev != nil {
		prev.close()
	}
}

// takePendingLogin removes and returns the pending sign-in, if there is one.
func (s *Server) takePendingLogin() *pendingLogin {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	p := s.pending
	s.pending = nil
	return p
}

// closePendingLogin ends whatever sign-in is pending.
func (s *Server) closePendingLogin() {
	if p := s.takePendingLogin(); p != nil {
		p.close()
	}
}

// pendingPID is the process id of the pending sign-in, or zero.
func (s *Server) pendingPID() int {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.pending == nil || s.pending.cmd.Process == nil {
		return 0
	}
	return s.pending.cmd.Process.Pid
}

// failLogin ends a sign-in that could not be started and explains why.
func (s *Server) failLogin(p *pendingLogin, err error) error {
	explained := p.explain(err)
	s.closePendingLogin()
	return explained
}

// close ends the child and everything it spawned.
func (p *pendingLogin) close() {
	p.cancel()
	killGroup(p.cmd)
	_ = p.cmd.Wait()
}

// explain turns a read failure into what the user is told.
//
// A non-zero exit whose stdout is not a document is a supported path rather than
// a defensive one: argv the command cannot parse fails before the flags that say
// how to render a failure are established. The exit status is reported and the
// bytes are not — they are the child's, and a child is not a trusted source of
// prose.
func (p *pendingLogin) explain(err error) error {
	// The remaining output may be a failure envelope, which is the declared way
	// a refusal is reported.
	if envelope, ok := readEnvelope(p.out); ok {
		return envelope
	}
	if !errors.Is(err, errUnreadableLogin) {
		return err
	}
	if werr := p.cmd.Wait(); werr != nil {
		var exit *exec.ExitError
		if errors.As(werr, &exit) {
			return fmt.Errorf("the sign-in failed (exit %d); run `formae login --hosted` to see why",
				exit.ExitCode())
		}
	}
	return errors.New("the sign-in produced no result formae could read; run `formae login --hosted` to see why")
}

// killGroup ends the child's whole process group, so an auth plugin it spawned
// cannot outlive it holding the loopback port the next flow needs.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// errUnreadableLogin is a document this build could not read.
var errUnreadableLogin = errors.New("unreadable sign-in output")

// readDocument reads one JSON document from the child.
//
// A document longer than the bound makes Scan report an error rather than
// returning it, so an oversized document is refused without being held.
func readDocument(r *bufio.Scanner) ([]byte, error) {
	for r.Scan() {
		line := r.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue // a blank line is not a document.
		}
		// Bytes() aliases the scanner's buffer, which the next Scan reuses.
		return append([]byte(nil), line...), nil
	}
	return nil, errUnreadableLogin
}

// docKind is which of the three documents this is.
type docKind int

const (
	docUnknown docKind = iota
	docStarted
	docComplete
	docFailure
)

// classify reads just enough of a document to know which it is.
//
// Classifying at all is the point, rather than the order of the cases below: a
// failure envelope carries no phase field, so code that asked only "does this
// parse as a started document" got a yes — every field zero — and reported a
// sign-in whose URL was empty. That is what a missing auth plugin produces, and
// it is the ordinary failure before any flow opens rather than an exotic one.
//
// The cases are mutually exclusive for every document the producer emits, so
// their order is presentation and not protection.
func classify(line []byte) docKind {
	var probe struct {
		SchemaVersion int    `json:"schemaVersion"`
		Phase         string `json:"phase"`
		Code          string `json:"code"`
	}
	if err := json.Unmarshal(line, &probe); err != nil || probe.SchemaVersion != loginSchemaVersion {
		return docUnknown
	}
	switch {
	case probe.Code != "":
		return docFailure
	case probe.Phase == "complete":
		return docComplete
	case probe.Phase == "started":
		return docStarted
	default:
		return docUnknown
	}
}

// envelopeError renders a declared failure as the MCP's own text.
func envelopeError(line []byte) error {
	var env struct {
		Code    string `json:"code"`
		Details struct {
			PluginCode string `json:"pluginCode"`
		} `json:"details"`
	}
	if err := json.Unmarshal(line, &env); err != nil {
		return errUnreadableLogin
	}
	return errors.New(describeLoginFailure(env.Code, env.Details.PluginCode))
}

// decodeComplete reads a completion document, checking its version first.
func decodeComplete(line []byte) (completeDoc, error) {
	var done completeDoc
	if err := json.Unmarshal(line, &done); err != nil {
		return completeDoc{}, errUnreadableLogin
	}
	if done.SchemaVersion != loginSchemaVersion || done.Phase != "complete" {
		return completeDoc{}, errUnreadableLogin
	}
	return done, nil
}

// readEnvelope reads a declared failure, if the child emitted one.
//
// The producer's own message is decoded and never surfaced: it is built from an
// auth plugin's error string, and a Pkl failure quotes profile source lines,
// which for a classic profile can mean an inline password. The MCP renders its
// own text from the code.
func readEnvelope(r *bufio.Scanner) (error, bool) {
	line, err := readDocument(r)
	if err != nil {
		return nil, false
	}
	if classify(line) != docFailure {
		return nil, false
	}
	return envelopeError(line), true
}

// describeLoginFailure renders a declared code as the MCP's own text.
func describeLoginFailure(code, pluginCode string) string {
	switch code {
	case "plugin_missing":
		return "signing in to the hosted platform needs the oidc auth plugin, which is not installed. " +
			"Install it with `pelmgr install oidc`, then try again"
	case "auth_failed":
		switch pluginCode {
		case "session_expired":
			return "the sign-in did not complete because the session expired; start it again and finish in the browser"
		case "not_logged_in":
			return "the sign-in did not complete; start it again and finish in the browser"
		case "issuer_unreachable":
			return "the identity provider could not be reached; try again shortly"
		case "unsupported":
			return "the auth plugin does not support this sign-in"
		}
		return "the sign-in was refused; run `formae login --hosted` to see why"
	case "ambiguous_profile":
		return "more than one profile exists, so formae cannot tell which installation you meant"
	default:
		return "the sign-in did not complete; run `formae login --hosted` to see why"
	}
}

// renderStarted tells the caller what to put in front of the user.
func renderStarted(d startedDoc) string {
	if d.Method == "device" {
		return fmt.Sprintf(
			"Sign-in started. Show the user this and wait for them:\n\n"+
				"  Visit %s\n  Enter the code: %s\n\n"+
				"The code expires in %d seconds. Call complete_login once they have finished.",
			d.VerificationURI, d.UserCode, d.ExpiresInSeconds)
	}
	return fmt.Sprintf(
		"Sign-in started. Show the user this URL and wait for them to open it:\n\n  %s\n\n"+
			"Call complete_login once they have finished signing in.",
		d.BrowserURL)
}

// renderComplete reports what a finished sign-in produced.
func renderComplete(d completeDoc) string {
	var b strings.Builder
	who := d.SubjectName
	if who == "" {
		who = d.Subject
	}

	if d.Status == "already_authenticated" {
		b.WriteString("Already signed in")
	} else {
		b.WriteString("Signed in")
	}
	if who != "" {
		b.WriteString(" as " + who)
	}
	b.WriteString(".\n")

	if len(d.Profiles.Created) > 0 {
		b.WriteString("Created profiles: " + strings.Join(d.Profiles.Created, ", ") + "\n")
	}
	if len(d.Profiles.Updated) > 0 {
		b.WriteString("Updated profiles: " + strings.Join(d.Profiles.Updated, ", ") + "\n")
	}
	if len(d.Profiles.Removed) > 0 {
		b.WriteString("Removed profiles: " + strings.Join(d.Profiles.Removed, ", ") + "\n")
	}
	if d.Active != "" {
		b.WriteString("Active profile: " + d.Active + "\n")
	}
	if len(d.Profiles.Created) == 0 && len(d.Profiles.Updated) == 0 && d.Active == "" {
		b.WriteString("No profiles were written. The signed-in account may cover no installations yet.\n")
	}
	for _, w := range d.Warnings {
		b.WriteString("Warning: " + w + "\n")
	}
	return b.String()
}

// loginState is the pending sign-in and the lock over it, embedded in Server.
type loginState struct {
	loginMu sync.Mutex
	pending *pendingLogin
	// runCtx is the server's own context, captured by Run. The login child is
	// tied to it rather than to a tool call's, so a sign-in survives the call
	// that began it.
	runCtx context.Context
}
