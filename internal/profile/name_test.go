package profile

import (
	"errors"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"default", "prod", "load-test", "a", "a1_b-2", "Prod1"}
	for _, n := range valid {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{"", "-rf", "--help", "_x", "../etc", "a/b", "a.b", "a b", "a$"}
	for _, n := range invalid {
		if err := ValidateName(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", n, err)
		}
	}
}
