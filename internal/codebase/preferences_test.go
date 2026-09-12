package codebase

import (
	"bytes"
	"context"
	"os"
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

func TestPreferenceDoesNotRewriteCodebaseRegistry(t *testing.T) {
	r := Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	legacy := []byte(`{"version":1,"bindings":[]}`)
	if err := os.WriteFile(r.Path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetDriftPreference(context.Background(), hosted("000000000000000000000000001"), "auto_absorb_external"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(r.Path)
	if err != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("changed legacy registry: %s %v", got, err)
	}
}

func TestUnknownFuturePreferencePreservesSourceSelection(t *testing.T) {
	r := Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	id := hosted("000000000000000000000000001")
	err := r.preferencesRegistry().update(context.Background(), func(d *document) error {
		d.Preferences = []driftPreferenceRecord{{Identity: id, Mode: "future-mode"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pref, err := r.DriftPreference(context.Background(), id)
	if err != nil || pref.Mode != "prompt" || !pref.Unavailable {
		t.Fatalf("unsafe future fallback: %+v %v", pref, err)
	}
	selection, err := r.Select(context.Background(), id, Request{})
	if err != nil || selection.Mode != "none" {
		t.Fatalf("preference blocked source: %+v %v", selection, err)
	}
}
