// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package codebase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	ModeCodebase     = "codebase"
	ModeNone         = "none"
	ModeSelect       = "selection_required"
	ModeUnconfigured = "unconfigured"
)

type Request struct {
	Mode             string
	BindingID        string
	WorkingDirectory string
	Stacks           []string
}

type Candidate struct {
	Binding   Binding `json:"binding"`
	Available bool    `json:"available"`
	Problem   string  `json:"problem,omitempty"`
}

type Selection struct {
	Mode       string      `json:"mode"`
	Binding    *Binding    `json:"binding,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Select uses a caller-provided workspace, never the server process directory.
// A selection is not authorization; callers still validate the current routed
// connection, operation's complete stack set, and agent capability.
func (r Registry) Select(ctx context.Context, identity Identity, request Request) (Selection, error) {
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return Selection{}, err
	}
	stacks, err := normalizeStacks(request.Stacks)
	if err != nil {
		return Selection{}, err
	}
	entries, err := r.List(ctx)
	if err != nil {
		return Selection{}, err
	}
	if request.Mode != "" && request.Mode != ModeNone && request.Mode != ModeCodebase {
		return Selection{}, errors.New("unknown codebase selection mode")
	}
	if request.Mode == ModeNone {
		if request.BindingID != "" {
			return Selection{}, errors.New("no-codebase mode cannot name a project binding")
		}
		return Selection{Mode: ModeNone}, nil
	}
	if request.Mode == ModeCodebase && request.BindingID == "" {
		return Selection{}, errors.New("explicit codebase mode requires a binding ID")
	}
	if request.BindingID != "" {
		for _, b := range entries {
			if b.ID == request.BindingID {
				if b.Identity != identity {
					return Selection{}, errors.New("codebase binding belongs to another installation")
				}
				return selectBinding(b, stacks)
			}
		}
		return Selection{}, fmt.Errorf("codebase binding %q is not registered", request.BindingID)
	}
	candidates := []Candidate{}
	for _, b := range entries {
		if b.Identity == identity {
			c := Candidate{Binding: b, Available: true}
			if err := available(b); err != nil {
				c.Available = false
				c.Problem = err.Error()
			}
			candidates = append(candidates, c)
		}
	}
	if request.WorkingDirectory != "" {
		if !filepath.IsAbs(request.WorkingDirectory) {
			return Selection{}, errors.New("working directory must be absolute")
		}
		workspace, err := filepath.EvalSymlinks(request.WorkingDirectory)
		if err != nil {
			return Selection{}, fmt.Errorf("working directory unavailable: %w", err)
		}
		info, err := os.Stat(workspace)
		if err != nil {
			return Selection{}, err
		}
		if !info.IsDir() {
			return Selection{}, errors.New("working directory is not a directory")
		}
		var nearest *Binding
		for _, c := range candidates {
			if inside(c.Binding.Path, workspace) && (nearest == nil || len(c.Binding.Path) > len(nearest.Path)) {
				b := c.Binding
				nearest = &b
			}
		}
		if nearest != nil {
			return selectBinding(*nearest, stacks)
		}
	}
	if len(candidates) > 0 {
		return Selection{Mode: ModeSelect, Candidates: candidates}, nil
	}
	if identity.Kind == "hosted" {
		return Selection{Mode: ModeNone}, nil
	}
	return Selection{Mode: ModeUnconfigured}, nil
}

func selectBinding(b Binding, stacks []string) (Selection, error) {
	if err := available(b); err != nil {
		return Selection{}, err
	}
	if len(b.Stacks) > 0 {
		for _, stack := range stacks {
			if !slices.Contains(b.Stacks, stack) {
				return Selection{}, fmt.Errorf("stack %q is outside the selected codebase scope", stack)
			}
		}
	}
	return Selection{Mode: ModeCodebase, Binding: &b}, nil
}

func available(b Binding) error {
	path, err := projectPath(b.Path)
	if err != nil {
		return fmt.Errorf("registered project %s is unavailable: %w", b.Path, err)
	}
	if path != b.Path {
		return errors.New("registered project path now resolves elsewhere; select and register its new location explicitly")
	}
	return nil
}

func inside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// ValidateFile rechecks the persisted binding, actual operation stack scope and
// symlink-resolved existing file. This is contextual validation, not a sandbox
// against another process replacing files after this call.
func (r Registry) ValidateFile(ctx context.Context, identity Identity, id, path string, stacks []string) (string, error) {
	if id == "" {
		return "", errors.New("file context requires a project binding")
	}
	selected, err := r.Select(ctx, identity, Request{Mode: ModeCodebase, BindingID: id, Stacks: stacks})
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("forma file path must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !inside(selected.Binding.Path, canonical) {
		return "", errors.New("forma file is outside the selected project")
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("forma path is not a regular file")
	}
	return canonical, nil
}
