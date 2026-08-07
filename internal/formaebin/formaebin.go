// Package formaebin selects which formae binary the MCP shells out to.
package formaebin

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Mode is the connection mode a call runs in. Classic talks to the user's own
// agent; Hosted (not yet resolved in this phase) talks to the managed platform.
type Mode int

const (
	Classic Mode = iota
	Hosted
)

// knownClassicLocations are probed, in order, when formae is not on PATH.
var knownClassicLocations = []string{
	"/opt/pel/bin/formae",
	"/usr/local/bin/formae",
}

// BinResolver picks the formae binary path for a mode. Filesystem access is
// injected so the logic is unit-testable.
type BinResolver struct {
	BundledPath string
	LookPath    func(string) (string, error)
	Exists      func(string) bool
}

// NewBinResolver wires a BinResolver to the real filesystem.
func NewBinResolver() BinResolver {
	return BinResolver{
		BundledPath: bundledFormaePath(),
		LookPath:    exec.LookPath,
		Exists:      func(p string) bool { _, err := os.Stat(p); return err == nil },
	}
}

// Resolve returns the formae path for the mode. Hosted always uses the bundled,
// fleet-matched binary. Classic prefers the user's own formae (PATH, then known
// locations) and falls back to the bundle only when none is installed.
func (b BinResolver) Resolve(mode Mode) string {
	if mode == Hosted {
		return b.BundledPath
	}
	if p, err := b.LookPath("formae"); err == nil && p != "" {
		return p
	}
	for _, loc := range knownClassicLocations {
		if b.Exists(loc) {
			return loc
		}
	}
	return b.BundledPath
}

// bundledFormaePath is FORMAE_BUNDLED_BIN if set, else a formae next to the MCP
// executable, else the bare name (last-ditch PATH lookup at exec time).
func bundledFormaePath() string {
	if p := os.Getenv("FORMAE_BUNDLED_BIN"); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "formae")
	}
	return "formae"
}
