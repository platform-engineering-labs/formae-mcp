package profile

import (
	"fmt"
	"regexp"
)

// nameRe enforces a leading alphanumeric, then alphanumerics, hyphens, or
// underscores. It is intentionally stricter than the CLI's ^[a-zA-Z0-9_-]+$:
// the leading-alphanumeric rule blocks both path traversal (../) and CLI
// argument injection (a name like --help would otherwise be parsed as a flag
// when passed positionally to exec.Command).
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidateName returns ErrInvalidName if name is not a safe profile name.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}
