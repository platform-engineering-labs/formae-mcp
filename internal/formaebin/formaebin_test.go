package formaebin

import (
	"os/exec"
	"testing"
)

func resolverWith(env map[string]string, lookPath func(string) (string, error)) BinResolver {
	return BinResolver{
		Getenv:   func(k string) string { return env[k] },
		LookPath: lookPath,
	}
}

func TestResolve(t *testing.T) {
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	onPath := func(string) (string, error) { return "/usr/bin/formae", nil }

	cases := []struct {
		name     string
		env      map[string]string
		lookPath func(string) (string, error)
		want     string
	}{
		{
			"the launcher's choice wins",
			map[string]string{EnvBin: "/opt/pel/bin/formae"},
			onPath,
			"/opt/pel/bin/formae",
		},
		{
			"falls back to PATH when launched without the launcher",
			nil,
			onPath,
			"/usr/bin/formae",
		},
		{
			"falls back to the bare name so exec reports it is missing",
			nil,
			notFound,
			"formae",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolverWith(c.env, c.lookPath).Resolve(); got != c.want {
				t.Fatalf("Resolve() = %q, want %q", got, c.want)
			}
		})
	}
}

// Managed decides whether an upgrade needs sudo, so anything other than the
// launcher's explicit "1" must read as the user's own install.
func TestManaged(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"0", false},
		{"", false},
		{"true", false},
	}
	for _, c := range cases {
		t.Run("value="+c.value, func(t *testing.T) {
			r := resolverWith(map[string]string{EnvManaged: c.value}, nil)
			if got := r.Managed(); got != c.want {
				t.Fatalf("Managed() with %q = %v, want %v", c.value, got, c.want)
			}
		})
	}
}
