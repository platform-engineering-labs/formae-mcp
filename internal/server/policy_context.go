// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"context"
	"fmt"
	"os"

	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

type policyCallKey struct{}
type policyCall struct {
	ec     execctx.Context
	client *FormaeClient
	source *tools.SourceContext
}

func (s *Server) policyWorkspace(ctx context.Context, profile string, source *tools.SourceContext, file string) (context.Context, string, destination, error) {
	if source == nil && profile == "" {
		cwd, err := os.Getwd()
		return ctx, cwd, destination{}, err
	}
	ec, err := s.resolveCtx(ctx, profile)
	if err != nil {
		return ctx, "", destination{}, err
	}
	c, err := s.newClient(ec)
	if err != nil {
		return ctx, "", resolved(ec), err
	}
	call := &policyCall{ec: ec, client: c, source: source}
	ctx = context.WithValue(ctx, policyCallKey{}, call)
	if source == nil {
		cwd, err := os.Getwd()
		return ctx, cwd, resolved(ec), err
	}
	if file == "" {
		return ctx, "", resolved(ec), fmt.Errorf("selected policy source context requires explicit forma_file")
	}
	raw, err := s.evaluateSource(ctx, ec, c, source, file)
	if err != nil {
		return ctx, "", reached(ec, c), err
	}
	if err := s.validateSourceContext(ctx, ec, source, file, raw); err != nil {
		return ctx, "", reached(ec, c), err
	}
	root := source.TemporaryDirectory
	if source.Mode == codebase.ModeCodebase {
		registry, err := s.codebaseRegistry()
		if err != nil {
			return ctx, "", reached(ec, c), err
		}
		identity, err := codebase.IdentityForConnection(ec.Conn)
		if err != nil {
			return ctx, "", reached(ec, c), err
		}
		selection, err := registry.Select(ctx, identity, codebase.Request{Mode: codebase.ModeCodebase, BindingID: source.BindingID})
		if err != nil {
			return ctx, "", reached(ec, c), err
		}
		root = selection.Binding.Path
	}
	return ctx, root, reached(ec, c), nil
}

func (s *Server) policyEval(ctx context.Context) EvalFunc {
	if call, ok := ctx.Value(policyCallKey{}).(*policyCall); ok {
		return func(path string) ([]byte, error) {
			raw, err := s.evaluateSource(ctx, call.ec, call.client, call.source, path)
			if err != nil {
				return nil, err
			}
			// No resolver predicate may use a scoped file's evaluated contents until
			// its entire evaluated stack membership has passed context validation.
			if err := s.validateSourceContext(ctx, call.ec, call.source, path, raw); err != nil {
				return nil, err
			}
			return raw, nil
		}
	}
	return currentEvalFunc(s.formaeBin())
}
