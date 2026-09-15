// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package codebase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
)

func project(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "PklProject"), []byte("amends \"pkl:Project\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRegistryRefusesCorruptDataWithoutOverwriting(t *testing.T) {
	for _, contents := range []string{"{", `{"version":99,"bindings":[]}`, `{"version":1,"bindings":null}`} {
		t.Run(contents, func(t *testing.T) {
			root := t.TempDir()
			r := Registry{Path: filepath.Join(root, "registry.json")}
			if err := os.WriteFile(r.Path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := r.List(context.Background()); err == nil {
				t.Fatal("corruption read as empty")
			}
			if _, err := r.Register(context.Background(), project(t, filepath.Join(root, "project")), hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa"), nil); err == nil {
				t.Fatal("corruption overwritten")
			}
			data, err := os.ReadFile(r.Path)
			if err != nil || string(data) != contents {
				t.Fatal("failed write changed data")
			}
		})
	}
}

func TestRegistryIndependentProcessesDoNotLoseRegistrations(t *testing.T) {
	if path := os.Getenv("CODEBASE_TEST_REGISTRY"); path != "" {
		_, err := (Registry{Path: path}).Register(context.Background(), os.Getenv("CODEBASE_TEST_PROJECT"), hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa"), nil)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	root := t.TempDir()
	r := Registry{Path: filepath.Join(root, "config", "registry.json")}
	commands := make([]*exec.Cmd, 8)
	for i := range commands {
		p := project(t, filepath.Join(root, fmt.Sprint(i)))
		cmd := exec.Command(os.Args[0], "-test.run=^TestRegistryIndependentProcessesDoNotLoseRegistrations$")
		cmd.Env = append(os.Environ(), "CODEBASE_TEST_REGISTRY="+r.Path, "CODEBASE_TEST_PROJECT="+p)
		commands[i] = cmd
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := r.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 8 {
		t.Fatalf("lost registrations: got %d", len(entries))
	}
}

func TestRegistryRejectsOversizedDocumentEvenAfterValidJSON(t *testing.T) {
	r := Registry{Path: filepath.Join(t.TempDir(), "registry.json")}
	data := `{"version":1,"bindings":[]}` + strings.Repeat(" ", 1024*1024+10)
	if err := os.WriteFile(r.Path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.List(context.Background()); err == nil {
		t.Fatal("oversized registry accepted")
	}
}

func TestRegistryLockCancellationAndUnregister(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r := Registry{Path: filepath.Join(root, "registry.json")}
	p := project(t, filepath.Join(root, "project"))
	identity := hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa")
	b, err := r.Register(ctx, p, identity, []string{"prod"})
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRegistry(ctx, r.Path+".lock")
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err = r.Register(canceled, p, identity, []string{"dev"})
	unlock()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want cancellation, got %v", err)
	}
	entries, err := r.List(ctx)
	if err != nil || len(entries) != 1 || entries[0].Stacks[0] != "prod" {
		t.Fatalf("canceled write changed binding: %v %v", entries, err)
	}
	if err = r.Unregister(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	entries, err = (Registry{Path: r.Path}).List(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unregister not durable: %v %v", entries, err)
	}
	info, err := os.Stat(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("registry permissions %o", info.Mode().Perm())
	}
}

func TestIdentityUsesResolvedInstallationAndRefusesCredentialEndpoints(t *testing.T) {
	a, err := IdentityForConnection(config.Hosted{Endpoint: "https://cloud.formae.ai/", Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := IdentityForConnection(config.Hosted{Endpoint: "https://cloud.formae.ai", Installation: "3HzFPXfPDGhwLJJVtaHbmFs6vLa"})
	if err != nil || a != b {
		t.Fatal("equivalent hosted routes diverged")
	}
	classic, err := IdentityForConnection(config.Classic{URL: "http://localhost", Port: 49684})
	if err != nil || classic.Kind != "classic" || classic.Endpoint != "http://localhost:49684" || classic.Installation != "" {
		t.Fatalf("wrong classic scope: %+v %v", classic, err)
	}
	for _, conn := range []config.Connection{
		config.Hosted{Endpoint: "https://evil.example", Installation: a.Installation},
		config.Classic{URL: "http://user:secret@localhost", Port: 49684},
		config.Classic{URL: "http://localhost/?token=secret"},
		nil,
	} {
		if _, err := IdentityForConnection(conn); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}

func TestClassicIdentityDoesNotMergeDifferentRoutedBasePaths(t *testing.T) {
	a, err := IdentityForConnection(config.Classic{URL: "http://localhost:49684/api/"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := IdentityForConnection(config.Classic{URL: "http://localhost:49684/api"})
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("different routed classic endpoints merged")
	}
}

func hosted(id string) Identity {
	return Identity{Kind: "hosted", Endpoint: "https://cloud.formae.ai", Installation: id}
}

func TestRegistryPersistsDistinctInstallationsAndUpdatesOnlySelectedBinding(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	r := Registry{Path: filepath.Join(root, "config", "codebases.json")}
	p := project(t, filepath.Join(root, "project"))
	a, err := r.Register(ctx, p, hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLa"), []string{"prod", "dev", "prod"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Register(ctx, p, hosted("3HzFPXfPDGhwLJJVtaHbmFs6vLb"), nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := r.Register(ctx, p, a.Identity, []string{"staging"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || updated.ID != a.ID {
		t.Fatal("binding identity lost")
	}
	entries, err := (Registry{Path: r.Path}).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want two installations, got %v", entries)
	}
	for _, e := range entries {
		if e.ID == a.ID && (len(e.Stacks) != 1 || e.Stacks[0] != "staging") {
			t.Fatalf("scope not persisted: %v", e.Stacks)
		}
		if e.ID == b.ID && len(e.Stacks) != 0 {
			t.Fatal("other installation changed")
		}
	}
}
