// Package formaebin reports which formae binary the MCP shells out to.
//
// There is exactly one formae per machine. The launcher
// (scripts/start-mcp.sh) finds the user's own install and provisions one only
// when the machine has none, then exports the answer. Detection lives there and
// only there: two detectors in two languages can disagree, and that
// disagreement is how a machine ends up with two installs.
package formaebin

import (
	"os"
	"os/exec"
)

const (
	// EnvBin names the formae binary the launcher selected.
	EnvBin = "FORMAE_BIN"
	// EnvManaged is "1" when that binary is the copy we provisioned into the
	// user's own tree, which can therefore be upgraded without sudo.
	EnvManaged = "FORMAE_BIN_MANAGED"
)

// BinResolver reads the launcher's decision. The environment and PATH lookup
// are injected so the logic is unit-testable.
type BinResolver struct {
	Getenv   func(string) string
	LookPath func(string) (string, error)
}

// NewBinResolver wires a BinResolver to the real process environment.
func NewBinResolver() BinResolver {
	return BinResolver{Getenv: os.Getenv, LookPath: exec.LookPath}
}

// Resolve returns the formae binary to run. The PATH fallback covers running
// this binary directly, without the launcher; the bare name after it leaves
// exec to report a missing formae in its own words rather than failing on an
// empty path.
func (b BinResolver) Resolve() string {
	if p := b.Getenv(EnvBin); p != "" {
		return p
	}
	if p, err := b.LookPath("formae"); err == nil && p != "" {
		return p
	}
	return "formae"
}

// Managed reports whether the resolved binary is ours to upgrade. Only the
// launcher's explicit "1" counts: everything else is the user's own install,
// where an upgrade needs sudo and their consent.
func (b BinResolver) Managed() bool {
	return b.Getenv(EnvManaged) == "1"
}
