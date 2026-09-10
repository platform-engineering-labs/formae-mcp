// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package codebase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectionUsesExplicitContextAndNearestWorkspaceWithoutChangingDefaults(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r := Registry{Path: filepath.Join(root, "registry.json")}
	identity := hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa")
	selection, err := r.Select(ctx, identity, Request{})
	if err != nil || selection.Mode != ModeNone {
		t.Fatalf("fresh hosted: %v %v", selection, err)
	}
	outer, err := r.Register(ctx, project(t, filepath.Join(root, "outer")), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := r.Register(ctx, project(t, filepath.Join(outer.Path, "inner")), identity, []string{"dev"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		req       Request
		mode      string
		id        string
		wantError bool
	}{
		{"elsewhere", Request{}, ModeSelect, "", false},
		{"workspace", Request{WorkingDirectory: inner.Path, Stacks: []string{"dev"}}, ModeCodebase, inner.ID, false},
		{"explicit", Request{BindingID: outer.ID, WorkingDirectory: inner.Path}, ModeCodebase, outer.ID, false},
		{"none", Request{Mode: ModeNone, WorkingDirectory: inner.Path}, ModeNone, "", false},
		{"scope mismatch", Request{BindingID: inner.ID, Stacks: []string{"prod"}}, "", "", true},
		{"contradictory", Request{Mode: ModeNone, BindingID: outer.ID}, "", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := r.Select(ctx, identity, test.req)
			if test.wantError {
				if err == nil {
					t.Fatal("expected explicit context error")
				}
				return
			}
			if err != nil || got.Mode != test.mode {
				t.Fatalf("got %+v %v", got, err)
			}
			if test.id != "" && (got.Binding == nil || got.Binding.ID != test.id) {
				t.Fatalf("wrong project: %+v", got)
			}
		})
	}
	selection, err = r.Select(ctx, identity, Request{})
	if err != nil || selection.Mode != ModeSelect || len(selection.Candidates) != 2 {
		t.Fatalf("per-call selection leaked: %+v %v", selection, err)
	}
	if _, err = r.Select(ctx, hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLb"), Request{BindingID: inner.ID}); err == nil {
		t.Fatal("installation mismatch accepted")
	}
}

func TestMissingProjectIsVisibleAndExplicitSelectionFails(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r := Registry{Path: filepath.Join(root, "registry.json")}
	identity := hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa")
	b, err := r.Register(ctx, project(t, filepath.Join(root, "gone")), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(b.Path); err != nil {
		t.Fatal(err)
	}
	got, err := r.Select(ctx, identity, Request{})
	if err != nil || got.Mode != ModeSelect || len(got.Candidates) != 1 || got.Candidates[0].Available || got.Candidates[0].Problem == "" {
		t.Fatalf("missing project hidden: %+v %v", got, err)
	}
	if _, err = r.Select(ctx, identity, Request{BindingID: b.ID}); err == nil {
		t.Fatal("missing selected path accepted")
	}
	got, err = r.Select(ctx, identity, Request{Mode: ModeNone})
	if err != nil || got.Mode != ModeNone {
		t.Fatal("explicit no-codebase should still work")
	}
}

func TestFileValidationRejectsScopeAndSymlinkEscape(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r := Registry{Path: filepath.Join(root, "registry.json")}
	identity := hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa")
	b, err := r.Register(ctx, project(t, filepath.Join(root, "project")), identity, []string{"prod"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(b.Path, "main.pkl")
	outside := filepath.Join(root, "outside.pkl")
	for _, p := range []string{file, outside} {
		if err = os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(b.Path, "escape.pkl")
	if err = os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ValidateFile(ctx, identity, b.ID, file, []string{"prod"}); err != nil || got != file {
		t.Fatalf("valid file rejected: %q %v", got, err)
	}
	for _, p := range []string{outside, link} {
		if _, err = r.ValidateFile(ctx, identity, b.ID, p, []string{"prod"}); err == nil {
			t.Fatalf("escaped project: %s", p)
		}
	}
	if _, err = r.ValidateFile(ctx, identity, b.ID, file, []string{"dev"}); err == nil {
		t.Fatal("scope bypass")
	}
	alias := filepath.Join(root, "alias")
	if err = os.Symlink(b.Path, alias); err != nil {
		t.Fatal(err)
	}
	got, err := r.Select(ctx, identity, Request{WorkingDirectory: alias, Stacks: []string{"prod"}})
	if err != nil || got.Binding == nil || got.Binding.ID != b.ID {
		t.Fatalf("workspace alias lost: %+v %v", got, err)
	}
}
