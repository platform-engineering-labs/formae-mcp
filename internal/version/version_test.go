package version

import "testing"

func TestString_StampedOverrideWins(t *testing.T) {
	t.Cleanup(func() { version = "" })

	version = "1.2.3"
	if got := String(); got != "1.2.3" {
		t.Errorf("String() = %q, want %q", got, "1.2.3")
	}
}

func TestString_FallsBackToDev(t *testing.T) {
	t.Cleanup(func() { version = "" })

	// Unstamped: with no usable build info (go test binaries report the
	// "(devel)" placeholder for Main.Version), String() falls back to "dev".
	version = ""
	if got := String(); got != "dev" {
		t.Errorf("String() = %q, want %q", got, "dev")
	}
}
