// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// evaluatedStacks includes explicit empty stacks and membership on resources and
// generators. A caller-supplied discovery scope is never used as this proof.
func evaluatedStacks(raw []byte) ([]string, error) {
	var forma struct {
		Stacks     []struct{ Label string }
		Resources  []struct{ Stack string }
		Generators []struct{ Stack string }
	}
	if err := json.Unmarshal(raw, &forma); err != nil {
		return nil, fmt.Errorf("decode evaluated stack scope: %w", err)
	}
	labels := map[string]bool{}
	for _, stack := range forma.Stacks {
		if stack.Label == "" {
			return nil, fmt.Errorf("evaluated stack has no label")
		}
		labels[stack.Label] = true
	}
	add := func(stack string) {
		if stack == "" {
			stack = "default"
		}
		labels[stack] = true
	}
	for _, resource := range forma.Resources {
		add(resource.Stack)
	}
	for _, generator := range forma.Generators {
		add(generator.Stack)
	}
	result := make([]string, 0, len(labels))
	for label := range labels {
		result = append(result, label)
	}
	sort.Strings(result)
	return result, nil
}

// sourceContextError is a workflow boundary failure, not an ordinary partial
// Pkl module that a workspace resolver may skip.
type sourceContextError struct {
	path string
	err  error
}

func (e *sourceContextError) Error() string {
	return fmt.Sprintf("source context for %s: %v", e.path, e.err)
}
func (e *sourceContextError) Unwrap() error { return e.err }
func isSourceContextError(err error) bool {
	var failure *sourceContextError
	return errors.As(err, &failure)
}

func (s *Server) validateSourceContext(ctx context.Context, ec execctx.Context, source *tools.SourceContext, path string, raw []byte) (err error) {
	if source == nil {
		return nil
	} // Existing file callers remain compatible.
	defer func() {
		if err != nil {
			err = &sourceContextError{path: path, err: err}
		}
	}()
	registry, err := s.codebaseRegistry()
	if err != nil {
		return err
	}
	identity, err := codebase.IdentityForConnection(ec.Conn)
	if err != nil {
		return err
	}
	stacks, err := evaluatedStacks(raw)
	if err != nil {
		return err
	}
	switch source.Mode {
	case codebase.ModeCodebase:
		if source.TemporaryDirectory != "" {
			return fmt.Errorf("codebase context cannot name a temporary directory")
		}
		_, err = registry.ValidateFile(ctx, identity, source.BindingID, path, stacks)
		return err
	case codebase.ModeNone:
		if _, err := registry.Select(ctx, identity, codebase.Request{Mode: codebase.ModeNone}); err != nil {
			return err
		}
		return validateDisposable(identity, source, path, stacks)
	default:
		return fmt.Errorf("source context mode must be codebase or none")
	}
}

func (s *Server) evaluateSource(ctx context.Context, ec execctx.Context, c *FormaeClient, source *tools.SourceContext, path string) ([]byte, error) {
	if source == nil {
		return evalFormaFile(ctx, ec, path)
	}
	// Validate file identity and containment before evaluating user source, then
	// revalidate with the complete evaluated stack set before any submission.
	if err := s.validateSourceContext(ctx, ec, source, path, []byte(`{}`)); err != nil {
		return nil, err
	}
	if source.Mode == codebase.ModeNone {
		for _, capability := range []string{"desired-stack-extraction", "shared-drift-resolution"} {
			if err := c.requireCapability(ctx, capability); err != nil {
				return nil, err
			}
		}
	}
	if strings.HasSuffix(path, ".json") {
		return os.ReadFile(path)
	}
	// Machine JSON evaluation is local. A neutral embedded-schema config keeps
	// mutable profile aliases and their authentication plugins out of this path.
	directory, err := os.MkdirTemp("", "formae-local-eval-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	configPath := filepath.Join(directory, "config.pkl")
	if err := os.WriteFile(configPath, []byte("amends \"formae:/Config.pkl\"\n"), 0600); err != nil {
		return nil, err
	}
	cmd := commandWithContext(ctx, ec.FormaeBin, "eval", path, "--output-schema", "json", "--output-consumer", "machine", "--config", configPath)
	cmd.Dir = filepath.Dir(path)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("local formae eval failed: %s", safeSubprocessOutput(exitErr.Stderr, ec.Credential))
		}
		return nil, fmt.Errorf("local formae eval failed: %w", err)
	}
	return output, nil
}
