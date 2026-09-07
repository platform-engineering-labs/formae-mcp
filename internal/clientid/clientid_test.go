package clientid

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// newTestResolver returns a Resolver rooted at dir.
func newTestResolver(dir string) *Resolver {
	return &Resolver{
		Home:     func() (string, error) { return dir, nil },
		ReadFile: os.ReadFile,
	}
}

func writeIDFile(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, ".pel", "formae", "cli_client_id")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveReturnsTrimmedFileContent(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1\n")
	got, err := newTestResolver(dir).Resolve()
	if err != nil || got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestResolveAcceptsExactly64Bytes(t *testing.T) {
	dir := t.TempDir()
	id := strings.Repeat("a", 64)
	writeIDFile(t, dir, id)
	got, err := newTestResolver(dir).Resolve()
	if err != nil || got != id {
		t.Fatalf("got %q, %v; want the 64-byte id accepted", got, err)
	}
}

// A malformed ID file is a fault the caller must see. Substituting a
// placeholder would report this machine under an identity shared with every
// other machine that hit the same fault.
func TestResolveFailsOnMalformedContent(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"whitespace only":  "   \n",
		"embedded newline": "abc\ndef",
		"embedded space":   "abc def",
		"control char":     "abc\x01def",
		"DEL byte":         "abc\x7fdef",
		"non-ascii":        "abc\xc3\xa9def",
		// printable, so only the length bound rejects it
		"over 64 bytes": strings.Repeat("a", 65),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeIDFile(t, dir, content)
			got, err := newTestResolver(dir).Resolve()
			if err == nil {
				t.Fatalf("got %q with no error, want a failure", got)
			}
		})
	}
}

// The file is created by the formae CLI, which every tool call runs to resolve
// its connection before the ID is read. Reaching a missing file means that did
// not happen, so it is reported rather than papered over by writing one here:
// a second writer could disagree with the CLI on the identity.
func TestResolveFailsWhenTheIDFileIsAbsent(t *testing.T) {
	dir := t.TempDir()
	got, err := newTestResolver(dir).Resolve()
	if err == nil {
		t.Fatalf("got %q with no error, want a failure", got)
	}
	if !strings.Contains(err.Error(), "run any formae command") {
		t.Fatalf("error %q does not tell the user how to recover", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".pel", "formae", "cli_client_id")); !os.IsNotExist(statErr) {
		t.Fatal("resolving must not create the ID file")
	}
}

func TestResolveFailsWhenHomeUnavailable(t *testing.T) {
	r := &Resolver{
		Home:     func() (string, error) { return "", os.ErrNotExist },
		ReadFile: os.ReadFile,
	}
	if got, err := r.Resolve(); err == nil {
		t.Fatalf("got %q with no error, want a failure", got)
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
	first, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q then %q", first, second)
	}
	if reads != 1 {
		t.Fatalf("read the file %d times, want 1", reads)
	}
}

// A failure is not cached: the next call retries rather than holding an error
// for the life of the process.
func TestResolveDoesNotCacheFailure(t *testing.T) {
	dir := t.TempDir()
	r := newTestResolver(dir)
	if _, err := r.Resolve(); err == nil {
		t.Fatal("want a failure when the file is absent")
	}
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	got, err := r.Resolve()
	if err != nil || got != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
		t.Fatalf("got %q, %v; want the file to be picked up once it exists", got, err)
	}
}

func TestResolveConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	writeIDFile(t, dir, "2N3x8aQdLmVp0rGhTzYwBcKfJe1")
	r := newTestResolver(dir)
	var wg sync.WaitGroup
	got := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := r.Resolve()
			if err != nil {
				t.Error(err)
				return
			}
			got <- id
		}()
	}
	wg.Wait()
	close(got)
	for id := range got {
		if id != "2N3x8aQdLmVp0rGhTzYwBcKfJe1" {
			t.Fatalf("got %q", id)
		}
	}
}

func TestNewResolverWiresTheRealFilesystem(t *testing.T) {
	r := NewResolver()
	if r.Home == nil || r.ReadFile == nil {
		t.Fatal("NewResolver must wire every filesystem seam")
	}
}
