package profile

import (
	"path/filepath"
)

// ActiveProfile returns the validated active profile name, or ErrNotInitialized
// if no active pointer exists, or ErrInvalidName if the pointer is corrupt.
// It is a pure read: it never bootstraps or rewrites.
func ActiveProfile() (string, error) {
	dir, err := ResolveConfigDir()
	if err != nil {
		return "", err
	}
	name, err := readActive(dir)
	if err != nil {
		return "", ErrNotInitialized
	}
	if err := ValidateName(name); err != nil {
		return "", err
	}
	return name, nil
}

// ProfilePath returns the absolute path to a profile's PKL file after
// validating the name (path-traversal / argument-injection guard).
func ProfilePath(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := ResolveConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles", name+".pkl"), nil
}
