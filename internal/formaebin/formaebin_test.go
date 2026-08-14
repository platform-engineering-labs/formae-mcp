package formaebin

import (
	"os/exec"
	"testing"
)

func TestResolve(t *testing.T) {
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	found := func(p string) func(string) (string, error) {
		return func(string) (string, error) { return p, nil }
	}
	none := func(string) bool { return false }
	only := func(match string) func(string) bool {
		return func(p string) bool { return p == match }
	}

	cases := []struct {
		name     string
		lookPath func(string) (string, error)
		exists   func(string) bool
		want     string
	}{
		{"prefers the user's own formae on PATH", found("/usr/bin/formae"), none, "/usr/bin/formae"},
		{"falls back to a known install location", notFound, only("/opt/pel/bin/formae"), "/opt/pel/bin/formae"},
		{"falls back to the bundle when none is installed", notFound, none, "/bundle/formae"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := BinResolver{BundledPath: "/bundle/formae", LookPath: c.lookPath, Exists: c.exists}
			if got := r.Resolve(); got != c.want {
				t.Fatalf("Resolve() = %q, want %q", got, c.want)
			}
		})
	}
}
