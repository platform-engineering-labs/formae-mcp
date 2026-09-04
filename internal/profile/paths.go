package profile

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// readActive returns the trimmed contents of <dir>/active, or an error if the
// pointer file is absent/unreadable.
func readActive(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "active"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// hasUserConfig reports whether dir holds config worth preserving: a valid
// active pointer naming an existing profile, OR at least one profiles/*.pkl,
// OR a formae.conf.pkl (regular file or symlink).
func hasUserConfig(dir string) bool {
	if name, err := readActive(dir); err == nil {
		if ValidateName(name) == nil {
			if _, statErr := os.Stat(filepath.Join(dir, "profiles", name+".pkl")); statErr == nil {
				return true
			}
		}
	}
	// Regular files only, as the CLI counts them. Glob matched symlinks and
	// directories ending in .pkl too, and this predicate also decides which config
	// directory wins: a stray profiles/x.pkl symlink under the legacy path could
	// select it over an XDG directory holding the user's real profiles.
	if entries, err := os.ReadDir(filepath.Join(dir, "profiles")); err == nil {
		for _, e := range entries {
			if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".pkl") {
				return true
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "formae.conf.pkl")); err == nil {
		return true
	}
	return false
}

// ResolveConfigDir mirrors the formae CLI's config-dir rule (RFC-27) so the MCP
// and CLI never disagree about where profiles live. First match wins.
func ResolveConfigDir() (string, error) {
	if v := os.Getenv("FORMAE_CONFIG_DIR"); v != "" {
		if v == "~" || strings.HasPrefix(v, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			v = filepath.Join(home, strings.TrimPrefix(v, "~"))
		}
		return filepath.Abs(v)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	legacy := filepath.Join(home, ".config", "formae")

	var xdg string
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		xdg = filepath.Join(x, "formae")
	}

	if hasUserConfig(legacy) {
		if xdg != "" && hasUserConfig(xdg) {
			slog.Warn("both legacy and XDG formae config dirs are populated; using legacy. Set FORMAE_CONFIG_DIR to choose deliberately.",
				"legacy", legacy, "xdg", xdg)
		}
		return legacy, nil
	}
	if xdg != "" && hasUserConfig(xdg) {
		return xdg, nil
	}
	if xdg != "" {
		return xdg, nil
	}
	return legacy, nil
}
