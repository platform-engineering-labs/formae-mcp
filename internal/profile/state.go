package profile

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The MCP has to know what a machine holds before it resolves a connection,
// because resolving one decides the question: `formae connection resolve`
// bootstraps a classic localhost profile on a machine that has none, so by the
// time the MCP could observe "nothing here" it has created something — for a
// user who may have meant to use the hosted platform.
//
// Everything here is a pure read. Nothing bootstraps, migrates, or writes, and
// the tests assert the store is byte-for-byte unchanged after each call.
//
// This reproduces the CLI's rules rather than shelling out to it. The rules are
// already reproduced in this package, deliberately and under test (see
// ResolveConfigDir); a subprocess in front of every agent-backed call would also
// put this behind a version floor and behind whatever an unknown formae prints,
// on the one code path whose whole job is to work on a machine we know nothing
// about.

// StoreState is what a formae config directory holds.
type StoreState int

const (
	// Unconfigured means there is nothing the CLI would preserve: no profiles,
	// no legacy config, no usable active pointer. This is the state a machine
	// that has never run formae is in, and the only one where the MCP asks the
	// user whether they are self-hosting or using the hosted platform.
	Unconfigured StoreState = iota

	// NoActive means configuration exists but nothing usable is selected: either
	// no pointer at all, or one naming a profile that is gone. Resolution fails
	// in both, and the CLI has a good message for each, so the MCP names the
	// profiles it found rather than letting that become a connection error.
	NoActive

	// Ready means a profile is selected and resolution can proceed.
	Ready
)

func (s StoreState) String() string {
	switch s {
	case Unconfigured:
		return "unconfigured"
	case NoActive:
		return "no-active"
	case Ready:
		return "ready"
	default:
		return "unknown"
	}
}

// List returns the profile names present, sorted. It is a pure read: it never
// bootstraps, migrates, or writes. An absent profiles/ dir is an empty list and
// not an error, because a clean store is the state this exists to detect.
//
// The filter matches the CLI's: a regular file, a .pkl suffix, and a stem that
// passes ValidateName. The last of those is what excludes the dotfile temporaries
// `formae login` writes while publishing a profile, which exist for the length of
// one publication and are profiles at no point.
func List() ([]string, error) {
	dir, err := ResolveConfigDir()
	if err != nil {
		return nil, err
	}
	return listIn(dir)
}

func listIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "profiles"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".pkl")
		if !ok || ValidateName(name) != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// State reports what the store holds and the profiles in it, without touching it.
//
// Unconfigured is hasUserConfig's question and not List's, and the difference is
// load-bearing: a store whose only configuration is a legacy formae.conf.pkl has
// no profiles, so a check that only counted those would report a configured user
// as having nothing — and the remedy onboarding offers migrates that file. The
// two readings have to agree, so this defers to the same predicate.
func State() (StoreState, []string, error) {
	dir, err := ResolveConfigDir()
	if err != nil {
		return Unconfigured, nil, err
	}

	names, err := listIn(dir)
	if err != nil {
		return Unconfigured, nil, err
	}

	if !hasUserConfig(dir) {
		return Unconfigured, names, nil
	}

	// What follows mirrors the CLI's own initialization decision, because the
	// question is not "is something here" but "would the CLI resolve it". Two of
	// its outcomes recover on their own and must not be reported as needing the
	// user's hand.
	//
	// Measured against the real binary, one temp config dir per row:
	//
	//	store                       CLI resolves   this reports
	//	a valid pointer             yes            Ready
	//	a legacy formae.conf.pkl    yes            Ready
	//	an orphaned default.pkl     yes            Ready
	//	profiles, no pointer        no             NoActive
	//	a pointer to a gone profile no             NoActive
	//	nothing at all              yes*           Unconfigured
	//
	// * and that row is the whole point: the CLI resolves an empty store by
	// creating a classic localhost default, which is the decision the user has
	// not been asked about yet.
	if active, err := readActive(dir); err == nil && ValidateName(active) == nil {
		if _, err := os.Stat(profilePath(dir, active)); err == nil {
			return Ready, names, nil // the pointer names a profile that exists.
		}
		// A pointer naming a profile that is gone. The CLI stops here and asks
		// the user to choose, and so does this.
		return NoActive, names, nil
	}

	// No usable pointer. A legacy formae.conf.pkl is migrated into a default
	// profile and pointed at, and an orphaned default is adopted, so both resolve
	// without anyone being asked anything.
	if _, err := os.Lstat(filepath.Join(dir, "formae.conf.pkl")); err == nil {
		return Ready, names, nil
	}
	if _, err := os.Stat(profilePath(dir, "default")); err == nil {
		return Ready, names, nil
	}

	// Profiles, but no default and no pointer: the one state the CLI itself
	// refuses, naming the profiles it found.
	return NoActive, names, nil
}

// profilePath is where a profile of this name lives.
func profilePath(dir, name string) string {
	return filepath.Join(dir, "profiles", name+".pkl")
}
