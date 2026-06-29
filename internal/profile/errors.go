package profile

import "errors"

var (
	// ErrInvalidName indicates a profile name that fails ValidateName.
	ErrInvalidName = errors.New("invalid profile name")
	// ErrNotFound indicates a named profile file does not exist.
	ErrNotFound = errors.New("profile not found")
	// ErrNotInitialized indicates there is no active profile pointer.
	ErrNotInitialized = errors.New("no active profile")
)
