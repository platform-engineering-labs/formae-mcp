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
		mode     Mode
		lookPath func(string) (string, error)
		exists   func(string) bool
		want     string
	}{
		{"hosted always bundled", Hosted, found("/anything"), func(string) bool { return true }, "/bundle/formae"},
		{"classic prefers PATH", Classic, found("/usr/bin/formae"), none, "/usr/bin/formae"},
		{"classic falls to known location", Classic, notFound, only("/opt/pel/bin/formae"), "/opt/pel/bin/formae"},
		{"classic falls to bundle", Classic, notFound, none, "/bundle/formae"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := BinResolver{BundledPath: "/bundle/formae", LookPath: c.lookPath, Exists: c.exists}
			if got := r.Resolve(c.mode); got != c.want {
				t.Fatalf("Resolve(%v) = %q, want %q", c.mode, got, c.want)
			}
		})
	}
}
