package clientid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestResolver returns a Resolver whose home is dir and whose ensure-run is
// a no-op, counting invocations in *ensureCalls.
func newTestResolver(dir string, ensureCalls *int) *Resolver {
	return &Resolver{
		Home:     func() (string, error) { return dir, nil },
		ReadFile: os.ReadFile,
		EnsureRun: func(string) error {
			if ensureCalls != nil {
				*ensureCalls++
			}
			return nil
		},
	}
}

// writeIDFile creates <dir>/.pel/formae/cli_client_id with the given content.
func writeIDFile(t *testing.T, dir, content string) {
	t.Helper()
	idDir := filepath.Join(dir, ".pel", "formae")
	if err := os.MkdirAll(idDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(idDir, "cli_client_id"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveReturnsTrimmedFileContent(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1\n")
	r := newTestResolver(dir, nil)
	if got := r.Resolve("formae"); got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveRejectsInvalidContent(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"whitespace only":  " \n\t",
		"embedded newline": "abc\ndef",
		"embedded space":   "abc def",
		"control char":     "abc\x01def",
		"non-ascii":        "abc\xc3\xa9def",
		// printable so only the length bound fails
		"over 64 bytes": strings.Repeat("a", 65),
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeIDFile(t, dir, content)
			r := newTestResolver(dir, nil)
			if got := r.Resolve("formae"); got != Fallback {
				t.Fatalf("got %q, want fallback", got)
			}
		})
	}
}

func TestResolveMissingFileRunsEnsureThenRereads(t *testing.T) {
	dir := t.TempDir()
	r := &Resolver{
		Home:     func() (string, error) { return dir, nil },
		ReadFile: os.ReadFile,
		// simulates formae creating the ID file on any invocation
		EnsureRun: func(string) error {
			writeIDFile(t, dir, "2CreatedByEnsureRun00000001")
			return nil
		},
	}
	if got := r.Resolve("formae"); got != "2CreatedByEnsureRun00000001" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveFallsBackWhenEnsureRunDoesNotHelp(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	r := newTestResolver(dir, &calls)
	if got := r.Resolve("formae"); got != Fallback {
		t.Fatalf("got %q, want fallback", got)
	}
	if calls != 1 {
		t.Fatalf("ensure-run called %d times, want 1", calls)
	}
}

func TestResolveFallsBackWhenHomeUnavailable(t *testing.T) {
	r := &Resolver{
		Home:      func() (string, error) { return "", os.ErrNotExist },
		ReadFile:  os.ReadFile,
		EnsureRun: func(string) error { return nil },
	}
	if got := r.Resolve("formae"); got != Fallback {
		t.Fatalf("got %q, want fallback", got)
	}
}

func TestResolveCachesSuccessfulRead(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	reads := 0
	r := newTestResolver(dir, nil)
	realRead := r.ReadFile
	r.ReadFile = func(p string) ([]byte, error) {
		reads++
		return realRead(p)
	}
	first := r.Resolve("formae")
	second := r.Resolve("formae")
	if first != second || first != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q then %q", first, second)
	}
	if reads != 1 {
		t.Fatalf("file read %d times, want 1", reads)
	}
}

func TestResolveDoesNotCacheFallback(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	r := newTestResolver(dir, &calls)
	if got := r.Resolve("formae"); got != Fallback {
		t.Fatalf("got %q, want fallback", got)
	}
	// the file appearing later must win over a previous fallback
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	if got := r.Resolve("formae"); got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q, want file content", got)
	}
	if calls != 1 {
		t.Fatalf("ensure-run called %d times, want 1", calls)
	}
}

func TestResolveConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	r := newTestResolver(dir, nil)
	done := make(chan string, 8)
	for i := 0; i < 8; i++ {
		go func() { done <- r.Resolve("formae") }()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
			t.Fatalf("got %q", got)
		}
	}
}
