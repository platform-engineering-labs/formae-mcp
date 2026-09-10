package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

const maxClientLogRead = 64 << 10

// A CLI invocation appends to this fixed file. Its error may contain only a
// pointer to the file, notably when JSON-to-Pkl conversion fails. Never accept
// a path from an error message or a tool argument, and never collect old logs.
type clientLogSnapshot struct {
	path        string
	before      os.FileInfo
	unavailable bool
	credential  secret.Value
}

func snapshotClientLog(ctx context.Context, ec execctx.Context) *clientLogSnapshot {
	if _, ok := ctx.Value(reportCaptureKey{}).(*reportCapture); !ok {
		return nil
	}
	if _, ok := ec.Conn.(config.Hosted); !ok {
		return nil
	}
	s := &clientLogSnapshot{credential: ec.Credential}
	home, err := os.UserHomeDir()
	if err != nil {
		s.unavailable = true
		return s
	}
	s.path = filepath.Join(home, ".pel", "formae", "log", "client.log")
	s.before, err = os.Lstat(s.path)
	if err != nil && !os.IsNotExist(err) {
		s.unavailable = true
	}
	if s.before != nil && !s.before.Mode().IsRegular() {
		s.unavailable = true
	}
	return s
}

func (s *clientLogSnapshot) excerpt() string {
	if s == nil {
		return ""
	}
	const missing = "Local client.log excerpt unavailable."
	if s.unavailable {
		return missing
	}
	info, err := os.Lstat(s.path)
	if err != nil || !info.Mode().IsRegular() {
		return missing
	}
	start := int64(0)
	if s.before != nil {
		if !os.SameFile(s.before, info) || info.Size() < s.before.Size() {
			return "Local client.log rotated or shrank during the operation; excerpt unavailable."
		}
		start = s.before.Size()
	}
	length := info.Size() - start
	if length == 0 {
		return "No new local client.log entries for this invocation."
	}
	// Do not cut arbitrary raw logs at a bound: that can remove the delimiters
	// needed to recognize a secret. Omit an oversized window instead.
	if length > maxClientLogRead {
		return "Local client.log append exceeded the safe capture limit; excerpt omitted."
	}
	// Nonblocking avoids hanging on a FIFO substituted between Lstat and open.
	// Verify the opened file before reading any bytes from it.
	f, err := os.OpenFile(s.path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return missing
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return missing
	}
	data, err := io.ReadAll(io.NewSectionReader(f, start, length))
	if err != nil || int64(len(data)) != length {
		return missing
	}
	clean := scrubReport(string(data), s.credential)
	if len(clean) > 8000 {
		clean = reportExcerpt(clean, 8000) + "\n[sanitized excerpt truncated]"
	}
	return "Local client.log entries appended during this CLI invocation (concurrent CLI activity may also appear):\n" + clean
}

func (s *clientLogSnapshot) capture(ctx context.Context) {
	if s != nil {
		captureReportLocalDiagnostics(ctx, s.excerpt())
	}
}
