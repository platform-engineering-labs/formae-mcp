package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

type commandHTTPError struct {
	status int
	body   []byte
}

func (e *commandHTTPError) Error() string {
	return fmt.Sprintf("agent returned status %d: %s", e.status, e.body)
}

const driftDecisionGuidance = `Describe the infrastructure decision in either codebase mode: "A change was made outside formae. Do you want to keep it or revert it?" Identify the resource and actionable properties. For a patch say "An earlier formae patch changed this resource; keep that change as desired state or revert it?" Do not infer permission to accept unrelated drift from "add", "update", or "preserve existing labels". Keep requested edits separate and retain the original full declaration for resolution; do not edit source to pre-absorb drift. Use one combined soft reconcile with ObservationID and all absorb/revert Decisions, simulate to obtain ReviewID, then submit after ordinary confirmation with a stable IdempotencyKey. Never substitute force. Suggest an optional factual message in that final confirmation, e.g. "Keep external bucket label and add application label"; the user can accept, edit or clear it. Acceptance is recorded centrally; update a selected maintained codebase afterward.`

func (s *Server) applyErrorResult(ctx context.Context, ec execctx.Context, err error) *mcp.CallToolResult {
	result := errorResult(err)
	var remote *commandHTTPError
	if !errors.As(err, &remote) || remote.status != 409 {
		return result
	}
	var envelope map[string]any
	if json.Unmarshal(remote.body, &envelope) != nil || envelope["error"] != "ReconcileRejected" {
		return result
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return result
	}
	observation, _ := data["ObservationID"].(string)
	preference := codebase.DriftPreference{Mode: "prompt"}
	preferenceError := false
	id, idErr := codebase.IdentityForConnection(ec.Conn)
	registry, registryErr := s.codebaseRegistry()
	if idErr == nil && registryErr == nil {
		var readErr error
		preference, readErr = registry.DriftPreference(ctx, id)
		preferenceError = readErr != nil
	} else {
		preferenceError = true
	}
	if preferenceError {
		preference = codebase.DriftPreference{Mode: "prompt"}
	}
	workflow := map[string]any{"drift_preference": preference, "resolution_available": observation != "", "preference_unavailable": preferenceError}
	envelope["workflow"] = workflow
	result.StructuredContent = envelope
	guidance := driftDecisionGuidance
	if observation == "" {
		guidance += " This response has no ObservationID: the connected agent has not supplied the recorded resolution protocol. Check its capabilities/version and explain the limitation; do not retry with force or claim a local source edit has accepted the drift."
	} else if preference.Mode == "auto_absorb_external" {
		guidance += " The user opted into automatically keeping nonconflicting external changes. Only resources with ExternalChangesOnly=true are eligible. False/missing means ask: it can include a patch or incomplete history, even when the latest command is sync. For eligible resources, propose absorb through the same resolution simulation; the agent's three-way merge decides conflicts. On decision-edit-conflict ask the user; never change their edit to manufacture a conflict-free result. Show automatic acceptance in the final combined preview and message. Patches always require a decision."
	} else if !preference.Explicit && !preferenceError {
		guidance += " At the first external change, also offer whether to ask in future or automatically keep nonconflicting external changes; patches always remain manual. Save only the user's explicit choice with set_drift_preference. No answer leaves prompt as the default."
	}
	return withNotice(result, guidance)
}
