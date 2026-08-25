package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// evalFormaFile must run the binary from the resolved context, not a bare
// "formae", so one call makes one binary decision and eval and submit cannot
// disagree about which formae they used.
func TestEvalFormaFileUsesContextBinary(t *testing.T) {
	ec := execctx.Context{FormaeBin: "/nonexistent/marker-formae"}
	_, err := evalFormaFile(context.Background(), ec, "does-not-matter.pkl")
	if err == nil {
		t.Fatal("expected error from missing binary")
	}
	if !strings.Contains(err.Error(), "marker-formae") {
		t.Fatalf("error did not reference the context binary: %v", err)
	}
}

// recordingFormae writes an executable stub that records its argv to argsFile
// and then runs extra. It stands in for the formae CLI.
func recordingFormae(t *testing.T, extra string) (bin, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "formae")
	argsFile = filepath.Join(dir, "argv")
	body := "#!/bin/sh\nprintf '%s' \"$*\" > " + argsFile + "\n" + extra
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}
	return bin, argsFile
}

// serverWithStubResolver returns a server whose execution context is fixed, so
// handler tests exercise the resolved context rather than the machine's formae.
func serverWithStubResolver(t *testing.T, ec execctx.Context) *Server {
	t.Helper()
	s := New("")
	s.ctxResolver = &stubResolver{ec: ec}
	return s
}

// The subsequent CLI invocations in a call must address the profile the context
// resolved, not the global active pointer, which another CLI or MCP session can
// move underneath a running call.
func TestExtractPinsTheResolvedProfile(t *testing.T) {
	// The last argument is the output path formae extract is asked to write.
	bin, argsFile := recordingFormae(t, "for a in \"$@\"; do last=\"$a\"; done\necho 'extracted' > \"$last\"\n")

	s := serverWithStubResolver(t, execctx.Context{
		ProfileName: "resolved-active",
		Conn:        config.Classic{URL: "http://localhost", Port: 49684},
		FormaeBin:   bin,
	})

	res, _, err := s.handleExtractResources(context.Background(), nil, tools.ExtractResourcesInput{Query: "type=bucket"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler returned error result: %s", res.Content[0].(interface{ String() string }))
	}

	argv, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading recorded argv: %v", err)
	}
	if !strings.Contains(string(argv), "--profile resolved-active") {
		t.Fatalf("extract argv did not pin the resolved profile: %s", argv)
	}
}

func TestEvalPinsTheResolvedProfile(t *testing.T) {
	bin, argsFile := recordingFormae(t, "echo '{}'\n")

	ec := execctx.Context{ProfileName: "resolved-active", FormaeBin: bin}
	if _, err := evalFormaFile(context.Background(), ec, "some.pkl"); err != nil {
		t.Fatalf("evalFormaFile: %v", err)
	}

	argv, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading recorded argv: %v", err)
	}
	if !strings.Contains(string(argv), "--profile resolved-active") {
		t.Fatalf("eval argv did not pin the resolved profile: %s", argv)
	}
}

// A cancelled tool call must stop the CLI it started. exec.Command alone would
// leave the handler waiting for the subprocess to finish on its own.
func TestExtractHonoursCancellation(t *testing.T) {
	bin, _ := recordingFormae(t, "sleep 30\n")

	s := serverWithStubResolver(t, execctx.Context{
		Conn:      config.Classic{URL: "http://localhost", Port: 49684},
		FormaeBin: bin,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan bool, 1)
	go func() {
		res, _, _ := s.handleExtractResources(ctx, nil, tools.ExtractResourcesInput{Query: "type=bucket"})
		done <- res.IsError
	}()

	select {
	case isErr := <-done:
		if !isErr {
			t.Fatal("extract with a cancelled context: expected an error result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("extract did not return on a cancelled context")
	}
}
