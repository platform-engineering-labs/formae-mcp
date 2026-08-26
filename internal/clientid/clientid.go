// Package clientid resolves the client identity the MCP sends to the agent.
//
// The formae CLI persists a per-machine ID at ~/.pel/formae/cli_client_id,
// created by the CLI itself. The MCP sends the same ID so commands issued
// through it are attributed to the same client as the user's own CLI. This
// package never creates the file: when it is missing or unreadable, it
// degrades to the Fallback constant.
package clientid

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Fallback is sent when no CLI client ID can be resolved. It matches the
// constant the MCP historically sent, so agents see no new identity when
// resolution fails.
const Fallback = "formae-mcp"

// maxIDLen bounds accepted IDs. A KSUID is 27 bytes; the bound is generous so
// a future formae ID format still passes without coupling to the exact shape.
const maxIDLen = 64

// Resolver resolves the CLI client ID. Filesystem access is injected so the
// logic is unit-testable. Safe for concurrent use.
type Resolver struct {
	Home     func() (string, error)
	ReadFile func(string) ([]byte, error)

	mu     sync.Mutex
	cached string
}

// NewResolver wires a Resolver to the real filesystem.
func NewResolver() *Resolver {
	return &Resolver{
		Home:     os.UserHomeDir,
		ReadFile: os.ReadFile,
	}
}

// Resolve returns the CLI client ID, or Fallback when it cannot be read. It
// never fails a caller: an unresolvable ID degrades to Fallback. A
// successfully read ID is cached for the process lifetime (the file never
// changes once written); a fallback is not cached, so a later call picks up
// the real file once it exists.
func (r *Resolver) Resolve() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != "" {
		return r.cached
	}
	if id, ok := r.read(); ok {
		r.cached = id
		return id
	}
	return Fallback
}

func (r *Resolver) read() (string, bool) {
	home, err := r.Home()
	if err != nil {
		return "", false
	}
	data, err := r.ReadFile(filepath.Join(home, ".pel", "formae", "cli_client_id"))
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(data))
	if !validID(id) {
		return "", false
	}
	return id, true
}

// validID reports whether id is safe to send as an HTTP header value: 1-64
// bytes of printable ASCII with no whitespace or control characters. Go's
// HTTP transport rejects requests whose header values contain control
// characters, so an unvalidated corrupt file would fail every command.
func validID(id string) bool {
	if len(id) == 0 || len(id) > maxIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] <= 0x20 || id[i] >= 0x7f {
			return false
		}
	}
	return true
}
