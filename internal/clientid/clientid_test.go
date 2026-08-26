package clientid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestResolver returns a Resolver rooted at dir.
func newTestResolver(dir string) *Resolver {
	return &Resolver{
		Home:     func() (string, error) { return dir, nil },
		ReadFile: os.ReadFile,
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
	r := newTestResolver(dir)
	if got := r.Resolve(); got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAcceptsExactly64Bytes(t *testing.T) {
	dir := t.TempDir()
	id := strings.Repeat("a", 64)
	writeIDFile(t, dir, id)
	r := newTestResolver(dir)
	if got := r.Resolve(); got != id {
		t.Fatalf("got %q, want the 64-byte id accepted", got)
	}
}

func TestResolveRejectsInvalidContent(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"whitespace only":  " \n\t",
		"embedded newline": "abc\ndef",
		"embedded space":   "abc def",
		"control char":     "abc\x01def",
		"DEL byte":         "abc\x7fdef",
		"non-ascii":        "abc\xc3\xa9def",
		// printable so only the length bound fails
		"over 64 bytes": strings.Repeat("a", 65),
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeIDFile(t, dir, content)
			r := newTestResolver(dir)
			if got := r.Resolve(); got != Fallback {
				t.Fatalf("got %q, want fallback", got)
			}
		})
	}
}

func TestResolveMissingFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	r := newTestResolver(dir)
	if got := r.Resolve(); got != Fallback {
		t.Fatalf("got %q, want fallback", got)
	}
}

func TestResolveFallsBackWhenHomeUnavailable(t *testing.T) {
	r := &Resolver{
		Home:     func() (string, error) { return "", os.ErrNotExist },
		ReadFile: os.ReadFile,
	}
	if got := r.Resolve(); got != Fallback {
		t.Fatalf("got %q, want fallback", got)
	}
}

func TestResolveCachesSuccessfulRead(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	reads := 0
	r := newTestResolver(dir)
	realRead := r.ReadFile
	r.ReadFile = func(p string) ([]byte, error) {
		reads++
		return realRead(p)
	}
	first := r.Resolve()
	second := r.Resolve()
	if first != second || first != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q then %q", first, second)
	}
	if reads != 1 {
		t.Fatalf("file read %d times, want 1", reads)
	}
}

func TestResolveDoesNotCacheFallback(t *testing.T) {
	dir := t.TempDir()
	r := newTestResolver(dir)
	if got := r.Resolve(); got != Fallback {
		t.Fatalf("got %q, want fallback", got)
	}
	// the file appearing later must win over a previous fallback
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	if got := r.Resolve(); got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q, want file content", got)
	}
}

func TestResolveConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	r := newTestResolver(dir)
	done := make(chan string, 8)
	for i := 0; i < 8; i++ {
		go func() { done <- r.Resolve() }()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
			t.Fatalf("got %q", got)
		}
	}
}

func TestNewResolverUsesRealHomeAndReadFile(t *testing.T) {
	r := NewResolver()
	if r.Home == nil || r.ReadFile == nil {
		t.Fatal("NewResolver must wire Home and ReadFile")
	}
	// NewResolver must never crash even against a real, likely-fileless home.
	if got := r.Resolve(); got == "" {
		t.Fatal("Resolve must never return an empty string")
	}
}
