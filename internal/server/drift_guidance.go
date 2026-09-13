package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
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
	preference, preferenceError := s.currentDriftPreference(ctx, ec)
	workflow := map[string]any{"drift_preference": preference, "resolution_available": observation != "", "preference_unavailable": preferenceError}
	envelope["workflow"] = workflow
	result.StructuredContent = envelope
	guidance := driftDecisionGuidance
	if observation == "" {
		guidance += " This response has no ObservationID: the connected agent has not supplied the recorded resolution protocol. Check its capabilities/version and explain the limitation; do not retry with force or claim a local source edit has accepted the drift."
	} else if preference.Mode == "auto_absorb" {
		guidance += " The user opted into automatically keeping nonconflicting changes, including formae patches, external changes and mixed history. Propose absorb for every actionable resource in this observation through the same resolution simulation; ExternalChangesOnly is descriptive, not an eligibility restriction for this preference. The agent three-way merge checks conflicts. On decision-edit-conflict ask the user; never discard their requested edit to manufacture success. Show automatic acceptance in the final combined preview and message, including deletions leaving desired state. Ordinary apply confirmation remains required."
	} else if preference.Mode == "auto_absorb_external" {
		guidance += " The user opted into automatically keeping nonconflicting external changes. Only resources with ExternalChangesOnly=true are eligible. False/missing means ask: it can include a patch or incomplete history, even when the latest command is sync. For eligible resources, propose absorb through the same resolution simulation; the agent's three-way merge decides conflicts. On decision-edit-conflict ask the user; never change their edit to manufacture a conflict-free result. Show automatic acceptance in the final combined preview and message, explicitly including any externally deleted resource leaving desired state. Patches always require a decision."
	} else if !preference.Explicit && !preferenceError {
		guidance += " First ask only the current keep/revert question. " + afterKeepPreferenceGuidance
	}
	return withNotice(result, guidance)
}

// Read consent again at preview time: another session may have saved a choice.
func (s *Server) currentDriftPreference(ctx context.Context, ec execctx.Context) (codebase.DriftPreference, bool) {
	preference := codebase.DriftPreference{Mode: "prompt"}
	preferenceError := false
	id, idErr := codebase.IdentityForConnection(ec.Conn)
	registry, registryErr := s.codebaseRegistry()
	if idErr == nil && registryErr == nil {
		var readErr error
		preference, readErr = registry.DriftPreference(ctx, id)
		preferenceError = readErr != nil || preference.Unavailable
	} else {
		preferenceError = true
	}
	if preferenceError {
		preference = codebase.DriftPreference{Mode: "prompt"}
	}
	return preference, preferenceError
}

const afterKeepPreferenceGuidance = `Only after the user explicitly chose keep, and only if you have not already asked this preference question during this operation, ask: "For future changes, should I keep asking, or automatically keep changes that do not conflict with your requested edits? This includes changes made outside formae, formae patches, and deletions; only conflicts require a keep/revert decision." A revert-only decision does not trigger this offer. Save only an explicit answer with set_drift_preference: prompt for keep asking, auto_absorb for automatically keep. Keeping this change is not consent to future automatic acceptance. No answer leaves prompt as the default; do not save a choice or block the current apply on an unanswered preference question. Ordinary apply confirmation remains required.`

func (s *Server) keepPreferenceNotice(ctx context.Context, ec execctx.Context, input tools.ApplyFormaInput, result []byte) string {
	if !input.Simulate || input.Mode != "reconcile" || input.Resolution == nil {
		return ""
	}
	kept := false
	for _, decision := range input.Resolution.Decisions {
		if decision.Action == "absorb" {
			kept = true
			break
		}
	}
	if !kept {
		return ""
	}
	var response struct{ Review struct{ ReviewID string } }
	if json.Unmarshal(result, &response) != nil || response.Review.ReviewID == "" {
		return ""
	}
	preference, unavailable := s.currentDriftPreference(ctx, ec)
	if unavailable || preference.Explicit {
		return ""
	}
	// The harness must distinguish an explicit keep from automatically chosen absorb.
	// A successful preview alone is not consent to future acceptance.
	return afterKeepPreferenceGuidance
}
