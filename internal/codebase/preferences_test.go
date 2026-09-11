package codebase

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDriftPreferencePersistsAndIsInstallationScoped(t *testing.T) {
	ctx := context.Background()
	r := Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	a, b := hosted("000000000000000000000000001"), hosted("000000000000000000000000002")
	initial, err := r.DriftPreference(ctx, a)
	if err != nil || initial.Mode != "prompt" || initial.Explicit {
		t.Fatalf("default: %+v %v", initial, err)
	}
	if _, err := r.SetDriftPreference(ctx, a, "auto_absorb_external"); err != nil {
		t.Fatal(err)
	}
	reopened := Registry{Path: r.Path}
	got, err := reopened.DriftPreference(ctx, a)
	if err != nil || got.Mode != "auto_absorb_external" || !got.Explicit {
		t.Fatalf("saved: %+v %v", got, err)
	}
	other, err := reopened.DriftPreference(ctx, b)
	if err != nil || other.Mode != "prompt" || other.Explicit {
		t.Fatalf("other installation: %+v %v", other, err)
	}
	if _, err := reopened.SetDriftPreference(ctx, a, "auto_absorb_everything"); err == nil {
		t.Fatal("accepted invalid preference")
	}
	got, err = reopened.SetDriftPreference(ctx, a, "prompt")
	if err != nil || !got.Explicit || got.Mode != "prompt" {
		t.Fatalf("explicit prompt: %+v %v", got, err)
	}
}
