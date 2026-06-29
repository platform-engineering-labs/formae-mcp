package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveConfigDir_ForcedWinsAndAbsolutized(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
	if got != dir {
		t.Errorf("expected %q, got %q", dir, got)
	}
}

func TestResolveConfigDir_LegacyBeatsXDG(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	// populate legacy with a profile
	writeFile(t, filepath.Join(home, ".config", "formae", "profiles", "default.pkl"), "x")
	// populate xdg too
	writeFile(t, filepath.Join(xdg, "formae", "profiles", "default.pkl"), "x")
	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "formae")
	if got != want {
		t.Errorf("expected legacy %q, got %q", want, got)
	}
}

func TestResolveConfigDir_XDGWhenLegacyEmpty(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeFile(t, filepath.Join(xdg, "formae", "profiles", "prod.pkl"), "x")
	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "formae")
	if got != want {
		t.Errorf("expected xdg %q, got %q", want, got)
	}
}

func TestResolveConfigDir_TildeSlash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORMAE_CONFIG_DIR", "~/sub")
	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "sub")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
}

func TestResolveConfigDir_TildeAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORMAE_CONFIG_DIR", "~")
	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Errorf("expected %q, got %q", home, got)
	}
}

func TestResolveConfigDir_TildeFoo_NotExpanded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORMAE_CONFIG_DIR", "~foo")
	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	// ~foo must NOT be tilde-expanded: result must end with /~foo, not <home>foo.
	want, _ := filepath.Abs("~foo")
	if got != want {
		t.Errorf("expected %q (abs of ~foo), got %q", want, got)
	}
}

func TestHasUserConfig(t *testing.T) {
	// empty dir -> false
	empty := t.TempDir()
	if hasUserConfig(empty) {
		t.Error("empty dir should not be user config")
	}
	// stale active naming missing profile, no profiles -> false
	stale := t.TempDir()
	writeFile(t, filepath.Join(stale, "active"), "ghost\n")
	if hasUserConfig(stale) {
		t.Error("stale active with no profiles should not be user config")
	}
	// has a profile -> true
	withProfile := t.TempDir()
	writeFile(t, filepath.Join(withProfile, "profiles", "default.pkl"), "x")
	if !hasUserConfig(withProfile) {
		t.Error("dir with a profile should be user config")
	}
}
