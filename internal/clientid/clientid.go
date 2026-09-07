// Package clientid resolves the client identity the MCP sends to the agent.
//
// The formae CLI keeps a per-machine ID at ~/.pel/formae/cli_client_id and
// creates it on any CLI invocation. The MCP sends the same ID so commands
// issued through it are attributed to the same client as the user's own CLI
// runs.
//
// This package only ever reads. Every tool call that needs an identity first
// resolves its connection through the formae CLI, and that invocation creates
// the file, so by the time the ID is read it exists. Writing it here as well
// would add a second writer for no gain: the CLI's own create is a stat
// followed by a non-atomic write, so two writers could disagree on the
// identity, and a create interrupted between reserving the path and filling it
// would leave an empty file that reads as malformed forever.
//
// There is also no fallback identity. A constant would be the same string on
// every machine, which makes `client:` queries unable to tell two
// installations apart and is indistinguishable from an older MCP that sent no
// real ID at all. When no ID can be resolved the caller gets an error and the
// tool call fails, rather than reporting under an identity that means nothing.
package clientid

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// maxIDLen bounds accepted IDs. A KSUID is 27 bytes; the bound is generous so
// a future formae ID format still passes without coupling to the exact shape.
const maxIDLen = 64

// Resolver resolves the CLI client ID. Filesystem access is injected so the
// logic is unit-testable. Safe for concurrent use.
type Resolver struct {
	Home     func() (string, error)
	ReadFile func(string) ([]byte, error)

	mu     sync.Mutex // guards cached; see internal/featuregate for the same pattern
	cached string
}

// NewResolver wires a Resolver to the real filesystem.
func NewResolver() *Resolver {
	return &Resolver{
		Home:     os.UserHomeDir,
		ReadFile: os.ReadFile,
	}
}

// Resolve returns the machine's formae client ID. It fails rather than
// substituting a placeholder: a missing or malformed ID file is a real fault
// the user should see, not something to paper over with an identity shared by
// every machine.
//
// A resolved ID is cached for the process lifetime, since the file does not
// change once written. A failure is not cached, so a repaired file is picked
// up by the next call.
func (r *Resolver) Resolve() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached != "" {
		return r.cached, nil
	}

	home, err := r.Home()
	if err != nil {
		return "", fmt.Errorf("cannot locate the home directory holding the formae client ID: %w", err)
	}
	path := filepath.Join(home, ".pel", "formae", "cli_client_id")

	data, err := r.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Resolving the connection runs the formae CLI, which creates this
			// file, so reaching here means that did not happen.
			return "", fmt.Errorf(
				"no formae client ID at %s; run any formae command to create it", path)
		}
		return "", fmt.Errorf("cannot read the formae client ID at %s: %w", path, err)
	}

	id := strings.TrimSpace(string(data))
	if !validID(id) {
		return "", fmt.Errorf(
			"the formae client ID at %s is malformed; delete the file and run any formae command to recreate it", path)
	}

	r.cached = id
	return id, nil
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
