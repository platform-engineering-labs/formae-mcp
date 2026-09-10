package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

func clientLogFixture(t *testing.T) (context.Context, execctx.Context, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".pel", "formae", "log", "client.log")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	ec := execctx.Context{Conn: config.Hosted{Endpoint: config.HostedOrigin, Installation: "3IzNhVWTOwLD9D8HtLtjq9Jb8ic"}, Credential: secret.New("Bearer fixture-token")}
	return context.WithValue(context.Background(), reportCaptureKey{}, &reportCapture{}), ec, path
}

func appendClientLog(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestClientLogCapturesOnlyNewSanitizedLines(t *testing.T) {
	ctx, ec, path := clientLogFixture(t)
	appendClientLog(t, path, "old unrelated log\n")
	snapshot := snapshotClientLog(ctx, ec)
	appendClientLog(t, path, "conversion failed: missing schema; password=hunter2 token=fixture-token\n")
	got := snapshot.excerpt()
	if !strings.Contains(got, "missing schema") || strings.Contains(got, "old unrelated") || strings.Contains(got, "hunter2") || strings.Contains(got, "fixture-token") {
		t.Fatalf("unsafe or incomplete excerpt: %s", got)
	}
}

func TestClientLogNewFileAndRotation(t *testing.T) {
	ctx, ec, path := clientLogFixture(t)
	snapshot := snapshotClientLog(ctx, ec)
	appendClientLog(t, path, "new conversion failure\n")
	if got := snapshot.excerpt(); !strings.Contains(got, "new conversion failure") {
		t.Fatal(got)
	}
	snapshot = snapshotClientLog(ctx, ec)
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	appendClientLog(t, path, "unrelated replacement\n")
	got := snapshot.excerpt()
	if strings.Contains(got, "unrelated replacement") || !strings.Contains(got, "rotated") {
		t.Fatal(got)
	}
}

func TestClientLogRejectsSymlinksAndExcessiveAppend(t *testing.T) {
	ctx, ec, path := clientLogFixture(t)
	other := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(other, []byte("not a log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	got := snapshotClientLog(ctx, ec).excerpt()
	if strings.Contains(got, "not a log") || !strings.Contains(got, "unavailable") {
		t.Fatal(got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotClientLog(ctx, ec)
	appendClientLog(t, path, strings.Repeat("x", 65537))
	if got := snapshot.excerpt(); !strings.Contains(got, "capture limit") || strings.Contains(got, strings.Repeat("x", 100)) {
		t.Fatal("oversized append should be omitted")
	}
}

func TestClientLogNotCapturedWithoutHostedReportingContext(t *testing.T) {
	ctx, ec, _ := clientLogFixture(t)
	if got := snapshotClientLog(context.Background(), ec); got != nil {
		t.Fatal("capture without reporting context")
	}
	ec.Conn = config.Classic{URL: "http://localhost"}
	if got := snapshotClientLog(ctx, ec); got != nil {
		t.Fatal("capture for self-hosted")
	}
}

func TestEvalFailureAddsLocalClientLog(t *testing.T) {
	ctx, ec, path := clientLogFixture(t)
	bin := filepath.Join(t.TempDir(), "formae")
	program := "#!/bin/sh\nprintf 'conversion schema failure\\n' >> \"$HOME/.pel/formae/log/client.log\"\nprintf 'generic evaluation error\\n' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(program), 0700); err != nil {
		t.Fatal(err)
	}
	ec.FormaeBin = bin
	_, err := evalFormaFile(ctx, ec, "test.pkl")
	if err == nil {
		t.Fatal("expected failed eval")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	c := ctx.Value(reportCaptureKey{}).(*reportCapture)
	if !strings.Contains(c.localDiagnostics, "conversion schema failure") {
		t.Fatalf("missing local log: %q", c.localDiagnostics)
	}
}

func TestExtractFailureAddsConversionLogToReportEvent(t *testing.T) {
	_, ec, _ := clientLogFixture(t)
	bin := filepath.Join(t.TempDir(), "formae")
	program := "#!/bin/sh\nprintf 'JSON-to-Pkl generator failed: missing schema; token=client-secret\\n' >> \"$HOME/.pel/formae/log/client.log\"\nprintf 'something went wrong during extraction; consult client.log\\n' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(program), 0700); err != nil {
		t.Fatal(err)
	}
	ec.FormaeBin = bin
	ec.ProfileName = "original"
	s := serverWithStubResolver(t, ec)
	req := &mcp.CallToolRequest{Session: new(mcp.ServerSession), Params: &mcp.CallToolParamsRaw{Name: "extract_resources"}}
	handler := s.captureReportEvent(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		res, _, err := s.handleExtractResources(ctx, req, tools.ExtractResourcesInput{Query: "label:example"})
		return res, err
	})
	result, err := handler(context.Background(), "tools/call", req)
	if err != nil {
		t.Fatal(err)
	}
	res := result.(*mcp.CallToolResult)
	if !res.IsError {
		t.Fatal("expected extraction error")
	}
	s.reportState.mu.Lock()
	defer s.reportState.mu.Unlock()
	if len(s.reportState.events) != 1 {
		t.Fatalf("expected one report event, got %d", len(s.reportState.events))
	}
	for _, event := range s.reportState.events {
		if !strings.Contains(event.diagnostics, "JSON-to-Pkl generator failed") || strings.Contains(event.diagnostics, "client-secret") {
			t.Fatalf("missing or unsafe diagnostics: %s", event.diagnostics)
		}
	}
}
