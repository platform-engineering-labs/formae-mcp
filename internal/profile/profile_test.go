package profile

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestActiveProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)

	// absent pointer -> ErrNotInitialized
	if _, err := ActiveProfile(); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}

	// valid pointer
	writeFile(t, filepath.Join(dir, "active"), "prod\n")
	got, err := ActiveProfile()
	if err != nil {
		t.Fatal(err)
	}
	if got != "prod" {
		t.Errorf("expected prod, got %q", got)
	}

	// malformed pointer -> ErrInvalidName
	writeFile(t, filepath.Join(dir, "active"), "../escape\n")
	if _, err := ActiveProfile(); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}

func TestProfilePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	got, err := ProfilePath("prod")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "profiles", "prod.pkl")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if _, err := ProfilePath("../escape"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected ErrInvalidName, got %v", err)
	}
}
