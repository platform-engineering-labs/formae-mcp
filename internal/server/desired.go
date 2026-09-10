// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

func (c *FormaeClient) commandDesiredDelta(ctx context.Context, id, clientID string) (json.RawMessage, error) {
	if err := c.requireCapability(ctx, "shared-drift-resolution"); err != nil {
		return nil, err
	}
	body, status, err := c.do(ctx, request{Method: http.MethodGet, Path: "/api/v1/commands/" + url.PathEscape(id) + "/desired-delta", Headers: map[string]string{"Client-ID": clientID}}, retryOnce)
	if err != nil {
		return nil, err
	}
	if err := c.unroutedIf(status); err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("agent returned status %d: %s", status, body)
	}
	var shape struct{ Partial bool }
	if err := json.Unmarshal(body, &shape); err != nil {
		return nil, err
	}
	if !shape.Partial {
		return nil, fmt.Errorf("agent returned desired delta without its required partial marker")
	}
	return body, nil
}

func (s *Server) handleCommandDesiredDelta(ctx context.Context, _ *mcp.CallToolRequest, input tools.CommandDesiredDeltaInput) (*mcp.CallToolResult, any, error) {
	if input.CommandID == "" {
		return errorResult(fmt.Errorf("command_id is required")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	clientID, err := s.clientID.Resolve()
	if err != nil {
		return nil, nil, err
	}
	raw, err := c.commandDesiredDelta(ctx, input.CommandID, clientID)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(raw)), nil, nil
}
