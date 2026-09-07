// Package formaebin selects which formae binary the MCP shells out to.
package formaebin

import (
	"os"
	"os/exec"
	"path/filepath"
)

// knownClassicLocations are probed, in order, when formae is not on PATH.
var knownClassicLocations = []string{
	"/opt/pel/bin/formae",
	"/usr/local/bin/formae",
}

// BinResolver picks the formae binary path. Filesystem access is injected so
// the logic is unit-testable.
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

// Resolve returns the formae binary path. There is one formae per machine: the
// user's own when installed (PATH, then known locations), and the bundled copy
// only when none is. Selection is not per call and not mode-dependent.
func (b BinResolver) Resolve() string {
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
