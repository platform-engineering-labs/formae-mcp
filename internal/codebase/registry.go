// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

// Package codebase stores local, explicitly opted-in infrastructure projects.
package codebase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Identity is connection identity, never a profile alias or credential.
// Hosted identities must come from the verified connection oracle. Classic
// identities are only endpoint-scoped; they cannot identify a replaced agent.
type Identity struct {
	Kind         string `json:"kind"`
	Endpoint     string `json:"endpoint"`
	Installation string `json:"installation,omitempty"`
}

type Binding struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Identity Identity `json:"identity"`
	Stacks   []string `json:"stacks,omitempty"`
}

// Registry owns no active project. Each operation selects its own binding.
type Registry struct{ Path string }
type document struct {
	Version  int       `json:"version"`
	Bindings []Binding `json:"bindings"`
}

func Default() (Registry, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Registry{}, err
	}
	return Registry{Path: filepath.Join(dir, "formae-mcp", "codebases.json")}, nil
}

func (r Registry) Register(ctx context.Context, path string, identity Identity, stacks []string) (Binding, error) {
	var result Binding
	canonical, err := projectPath(path)
	if err != nil {
		return result, err
	}
	identity, err = normalizeIdentity(identity)
	if err != nil {
		return result, err
	}
	stacks, err = normalizeStacks(stacks)
	if err != nil {
		return result, err
	}
	err = r.update(ctx, func(d *document) error {
		for i, b := range d.Bindings {
			if b.Path == canonical && b.Identity == identity {
				d.Bindings[i].Stacks = stacks
				result = d.Bindings[i]
				return nil
			}
		}
		if len(d.Bindings) >= 1024 {
			return errors.New("codebase registry has too many bindings")
		}
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return err
		}
		result = Binding{ID: hex.EncodeToString(id), Path: canonical, Identity: identity, Stacks: stacks}
		d.Bindings = append(d.Bindings, result)
		return nil
	})
	return result, err
}

func (r Registry) Unregister(ctx context.Context, id string) error {
	return r.update(ctx, func(d *document) error {
		for i, b := range d.Bindings {
			if b.ID == id {
				d.Bindings = append(d.Bindings[:i], d.Bindings[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("codebase binding %q is not registered", id)
	})
}

// List does not require registered directories to remain present. Selection
// reports missing directories rather than silently switching operating modes.
func (r Registry) List(ctx context.Context) ([]Binding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, err := r.read()
	if err != nil {
		return nil, err
	}
	return d.Bindings, nil
}

func (r Registry) read() (document, error) {
	empty := document{Version: 1, Bindings: []Binding{}}
	if !filepath.IsAbs(r.Path) {
		return empty, errors.New("registry path must be absolute")
	}
	f, err := os.Open(r.Path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, fmt.Errorf("read codebase registry: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil {
		return empty, fmt.Errorf("read codebase registry: %w", err)
	}
	if len(data) > 1024*1024 {
		return empty, errors.New("codebase registry exceeds size limit")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var d document
	if err := dec.Decode(&d); err != nil {
		return empty, fmt.Errorf("invalid codebase registry: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return empty, errors.New("invalid codebase registry: trailing data or oversized document")
	}
	if d.Version != 1 || d.Bindings == nil || len(d.Bindings) > 1024 {
		return empty, errors.New("invalid or unsupported codebase registry format")
	}
	ids := map[string]bool{}
	pairs := map[string]bool{}
	for _, b := range d.Bindings {
		id, err := hex.DecodeString(b.ID)
		if err != nil || len(id) != 16 || hex.EncodeToString(id) != b.ID || ids[b.ID] {
			return empty, errors.New("invalid or duplicate codebase binding ID")
		}
		ids[b.ID] = true
		identity, err := normalizeIdentity(b.Identity)
		if err != nil || identity != b.Identity {
			return empty, errors.New("invalid codebase installation identity")
		}
		if !filepath.IsAbs(b.Path) || filepath.Clean(b.Path) != b.Path {
			return empty, errors.New("invalid codebase project path")
		}
		stacks, err := normalizeStacks(b.Stacks)
		if err != nil || !slices.Equal(stacks, b.Stacks) {
			return empty, errors.New("invalid codebase stack scope")
		}
		pairBytes, _ := json.Marshal([]any{b.Path, b.Identity})
		pair := string(pairBytes)
		if pairs[pair] {
			return empty, errors.New("duplicate codebase installation binding")
		}
		pairs[pair] = true
	}
	return d, nil
}

func (r Registry) update(ctx context.Context, change func(*document) error) error {
	if !filepath.IsAbs(r.Path) {
		return errors.New("registry path must be absolute")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(r.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	unlock, err := lockRegistry(ctx, r.Path+".lock")
	if err != nil {
		return err
	}
	defer unlock()
	d, err := r.read()
	if err != nil {
		return err
	}
	if err = change(&d); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("codebase registry exceeds size limit")
	}
	temp, err := os.CreateTemp(dir, ".codebases-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	defer func() { _ = temp.Close() }()
	if _, err = temp.Write(data); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), r.Path); err != nil {
		return err
	}
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = folder.Close() }()
	// A failure here is an uncertain durable write; the complete new document
	// is already visible. Re-registering the same binding is idempotent.
	return folder.Sync()
}

func normalizeStacks(stacks []string) ([]string, error) {
	result := slices.Clone(stacks)
	for _, s := range result {
		if strings.TrimSpace(s) == "" || strings.ContainsRune(s, 0) {
			return nil, errors.New("stack scope contains an empty or invalid label")
		}
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}

func projectPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("project path must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("project directory unavailable: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project path is not a directory")
	}
	info, err = os.Stat(filepath.Join(canonical, "PklProject"))
	if err != nil {
		return "", fmt.Errorf("project requires PklProject: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("PklProject is not a regular file")
	}
	return filepath.Clean(canonical), nil
}
