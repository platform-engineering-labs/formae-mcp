// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

type codebaseContextResult struct {
	Identity        codebase.Identity        `json:"identity"`
	Selection       codebase.Selection       `json:"selection"`
	DriftPreference codebase.DriftPreference `json:"drift_preference"`
}

type codebaseListResult struct {
	Identity codebase.Identity    `json:"identity"`
	Bindings []codebase.Candidate `json:"bindings"`
}

func (s *Server) localCodebaseContext(ctx context.Context, profile string) (codebase.Registry, codebase.Identity, execctx.Context, error) {
	ec, err := s.resolveCtx(ctx, profile)
	if err != nil {
		return codebase.Registry{}, codebase.Identity{}, ec, err
	}
	identity, err := codebase.IdentityForConnection(ec.Conn)
	if err != nil {
		return codebase.Registry{}, codebase.Identity{}, ec, err
	}
	registry, err := s.codebaseRegistry()
	return registry, identity, ec, err
}

func codebaseReply(ec execctx.Context, value any, err error) (*mcp.CallToolResult, any, error) {
	var result *mcp.CallToolResult
	if err == nil {
		var encoded []byte
		encoded, err = json.Marshal(value)
		if err == nil {
			result = jsonResult(encoded)
		}
	}
	if err != nil {
		result = errorResult(err)
	}
	if ec.Conn != nil {
		result = attribute(resolved(ec), result)
	}
	return result, nil, nil
}

func (s *Server) handleCodebaseContext(ctx context.Context, _ *mcp.CallToolRequest, input tools.CodebaseContextInput) (*mcp.CallToolResult, any, error) {
	registry, identity, ec, err := s.localCodebaseContext(ctx, input.Profile)
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	selection, err := registry.Select(ctx, identity, codebase.Request{
		Mode: input.Mode, BindingID: input.BindingID, WorkingDirectory: input.WorkingDirectory, Stacks: input.Stacks,
	})
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	preference, err := registry.DriftPreference(ctx, identity)
	if err != nil {
		preference = codebase.DriftPreference{Mode: "prompt", Unavailable: true}
	}
	if err == nil && !preference.Unavailable {
		s.workflowTelemetry.capture(ctx, ec, identity, selection.Mode, preference, "mcp_workflow_context")
	}
	return codebaseReply(ec, codebaseContextResult{Identity: identity, Selection: selection, DriftPreference: preference}, nil)
}

func (s *Server) handleSetDriftPreference(ctx context.Context, _ *mcp.CallToolRequest, input tools.DriftPreferenceInput) (*mcp.CallToolResult, any, error) {
	registry, identity, ec, err := s.localCodebaseContext(ctx, input.Profile)
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	preference, err := registry.SetDriftPreference(ctx, identity, input.Mode)
	if err == nil {
		s.workflowTelemetry.capture(ctx, ec, identity, "", preference, "mcp_drift_preference_changed")
	}
	return codebaseReply(ec, preference, err)
}

func (s *Server) handleListCodebases(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	registry, identity, ec, err := s.localCodebaseContext(ctx, input.Profile)
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	selection, err := registry.Select(ctx, identity, codebase.Request{})
	bindings := selection.Candidates
	if bindings == nil {
		bindings = []codebase.Candidate{}
	}
	return codebaseReply(ec, codebaseListResult{Identity: identity, Bindings: bindings}, err)
}

func (s *Server) handleRegisterCodebase(ctx context.Context, _ *mcp.CallToolRequest, input tools.RegisterCodebaseInput) (*mcp.CallToolResult, any, error) {
	registry, identity, ec, err := s.localCodebaseContext(ctx, input.Profile)
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	binding, err := registry.Register(ctx, input.Path, identity, input.Stacks)
	return codebaseReply(ec, binding, err)
}

func (s *Server) handleUnregisterCodebase(ctx context.Context, _ *mcp.CallToolRequest, input tools.UnregisterCodebaseInput) (*mcp.CallToolResult, any, error) {
	registry, identity, ec, err := s.localCodebaseContext(ctx, input.Profile)
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	bindings, err := registry.List(ctx)
	if err != nil {
		return codebaseReply(ec, nil, err)
	}
	for _, binding := range bindings {
		if binding.ID != input.BindingID {
			continue
		}
		if binding.Identity != identity {
			return codebaseReply(ec, nil, fmt.Errorf("codebase binding belongs to another installation"))
		}
		err := registry.Unregister(ctx, input.BindingID)
		return codebaseReply(ec, map[string]string{"removed_binding_id": input.BindingID}, err)
	}
	return codebaseReply(ec, nil, fmt.Errorf("codebase binding %q is not registered", input.BindingID))
}
