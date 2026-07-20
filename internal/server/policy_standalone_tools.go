package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// policyInventoryItem mirrors apimodel.PolicyInventoryItem from the formae
// agent (GET /api/v1/policies). Config is the marshalled domain policy:
// TTL     -> {"Type":"ttl","Label":"...","TTLSeconds":3600,"OnDependents":"abort"}
// AutoRec -> {"Type":"auto-reconcile","Label":"...","IntervalSeconds":300}
type policyInventoryItem struct {
	Label          string          `json:"Label"`
	Type           string          `json:"Type"`
	Config         json.RawMessage `json:"Config"`
	AttachedStacks []string        `json:"AttachedStacks"`
}

// policyConfig is the union of both policy configs, for reading Config.
type policyConfig struct {
	TTLSeconds      int64  `json:"TTLSeconds"`
	OnDependents    string `json:"OnDependents"`
	IntervalSeconds int64  `json:"IntervalSeconds"`
}

// mcpPolicyType converts the agent's policy-type vocabulary to the MCP wire
// form. The agent uses a hyphen ("auto-reconcile"); the MCP tools use an
// underscore ("auto_reconcile") to match the shipped create_inline_policy
// contract. Only this direction is needed — nothing sends a type to the agent.
func mcpPolicyType(agentType string) string {
	if agentType == "auto-reconcile" {
		return "auto_reconcile"
	}
	return agentType
}

// fetchPolicies reads the agent's standalone policy inventory.
func (s *Server) fetchPolicies() ([]policyInventoryItem, error) {
	body, err := s.client.ListPolicies()
	if err != nil {
		return nil, fmt.Errorf("list policies from agent: %w", err)
	}
	var items []policyInventoryItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse policy inventory: %w", err)
	}
	return items, nil
}

// findPolicyByLabel returns the inventory entry for a label.
func findPolicyByLabel(items []policyInventoryItem, label string) (policyInventoryItem, bool) {
	for _, item := range items {
		if item.Label == label {
			return item, true
		}
	}
	return policyInventoryItem{}, false
}

// policyLabelsOf renders the known labels for an error message.
func policyLabelsOf(items []policyInventoryItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

// validateStandalonePolicyFields checks the shared policy field rules.
func validateStandalonePolicyFields(label, policyType string, ttlSeconds int64, onDependents string, intervalSeconds int64) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}
	switch policyType {
	case "ttl":
		if ttlSeconds <= 0 {
			return fmt.Errorf("ttl_seconds is required and must be > 0 when policy_type=ttl")
		}
		if onDependents != "" && onDependents != "abort" && onDependents != "cascade" {
			return fmt.Errorf("on_dependents must be 'abort' or 'cascade', got %q", onDependents)
		}
	case "auto_reconcile":
		if intervalSeconds <= 0 {
			return fmt.Errorf("interval_seconds is required and must be > 0 when policy_type=auto_reconcile")
		}
	default:
		return fmt.Errorf("policy_type must be 'ttl' or 'auto_reconcile', got %q", policyType)
	}
	return nil
}

func (s *Server) handleCreateStandalonePolicy(_ context.Context, _ *mcp.CallToolRequest, input tools.CreateStandalonePolicyInput) (*mcp.CallToolResult, any, error) {
	if err := validateStandalonePolicyFields(input.Label, input.PolicyType, input.TTLSeconds, input.OnDependents, input.IntervalSeconds); err != nil {
		return errorResult(err), nil, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errorResult(fmt.Errorf("getwd: %w", err)), nil, nil
	}

	// A label must be unique across the whole project, not just the target
	// file — stacks reference standalone policies by label alone, so two
	// declarations sharing one is an invalid project state. Check the whole
	// workspace before planning, since the declaration may live in a file
	// other than the one we are about to edit.
	if existing, err := resolveStandalonePolicyFile(cwd, input.Label, currentEvalFunc()); err == nil {
		out := tools.CreateStandalonePolicyOutput{
			FilePath:  existing,
			Operation: "noop",
			Notes: []string{fmt.Sprintf(
				"a standalone policy labelled %q is already declared in %s; labels must be unique across "+
					"the project. Updating a standalone in place is not supported — delete it and recreate it",
				input.Label, existing)},
		}
		body, err := json.Marshal(out)
		if err != nil {
			return errorResult(fmt.Errorf("marshal output: %w", err)), nil, nil
		}
		return jsonResult(body), nil, nil
	} else {
		var ambiguous *policySourceAmbiguousError
		if errors.As(err, &ambiguous) {
			return errorResult(err), nil, nil
		}
	}

	filePath := input.FormaFile
	if filePath == "" {
		resolved, err := resolveMainFormaFile(cwd, currentEvalFunc())
		if err != nil {
			return errorResult(err), nil, nil
		}
		filePath = resolved
	}

	if err := checkPolicySchemaSupport(filePath); err != nil {
		return errorResult(err), nil, nil
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return errorResult(fmt.Errorf("read %s: %w", filePath, err)), nil, nil
	}

	plan, err := planCreateStandalonePolicy(string(source), StandalonePolicySpec{
		Label:           input.Label,
		PolicyType:      input.PolicyType,
		TTLSeconds:      input.TTLSeconds,
		OnDependents:    input.OnDependents,
		IntervalSeconds: input.IntervalSeconds,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}

	out := tools.CreateStandalonePolicyOutput{
		FilePath:             filePath,
		Operation:            plan.Operation,
		PKLSnippet:           plan.PKLSnippet,
		InsertionAnchorStart: plan.AnchorStart,
		InsertionAnchorEnd:   plan.AnchorEnd,
		ImportsToAdd:         plan.ImportsToAdd,
		Notes:                plan.Notes,
	}
	body, err := json.Marshal(out)
	if err != nil {
		return errorResult(fmt.Errorf("marshal output: %w", err)), nil, nil
	}
	return jsonResult(body), nil, nil
}

func (s *Server) handleAttachStandalonePolicy(_ context.Context, _ *mcp.CallToolRequest, input tools.AttachStandalonePolicyInput) (*mcp.CallToolResult, any, error) {
	if input.Stack == "" {
		return errorResult(fmt.Errorf("stack is required")), nil, nil
	}
	if input.PolicyLabel == "" {
		return errorResult(fmt.Errorf("policy_label is required")), nil, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errorResult(fmt.Errorf("getwd: %w", err)), nil, nil
	}

	// Pre-check 1: identify the policy and its type. The agent inventory is
	// authoritative, but a policy declared in source and not yet applied is
	// legitimately absent from it — the create-then-attach workflow attaches
	// before the first apply. Fall back to the workspace source in that case
	// rather than refusing a documented flow.
	var notes []string
	items, fetchErr := s.fetchPolicies()
	if fetchErr != nil {
		items = nil
	}
	policyType := ""
	if item, known := findPolicyByLabel(items, input.PolicyLabel); known {
		policyType = mcpPolicyType(item.Type)
	} else {
		declType, found, err := standalonePolicyTypeFromWorkspace(cwd, input.PolicyLabel, currentEvalFunc())
		if err != nil {
			return errorResult(err), nil, nil
		}
		if !found {
			return errorResult(fmt.Errorf(
				"no standalone policy labelled %q is known to the agent or declared anywhere in the "+
					"workspace; known to the agent: %v. Declare it first with create_standalone_policy",
				input.PolicyLabel, policyLabelsOf(items))), nil, nil
		}
		policyType = declType
		notes = append(notes, fmt.Sprintf(
			"standalone policy %q is declared in source but not yet applied — the agent does not know it. "+
				"Apply the declaring file along with this attachment", input.PolicyLabel))
	}

	// Pre-check 2: no OTHER standalone of the same type may already be attached
	// to this stack — a stack may hold only one policy per type.
	for _, other := range items {
		if other.Label == input.PolicyLabel || mcpPolicyType(other.Type) != policyType {
			continue
		}
		for _, attached := range other.AttachedStacks {
			if attached == input.Stack {
				return errorResult(fmt.Errorf(
					"stack %q already has standalone policy %q of type %s attached; "+
						"a stack may hold only one policy per type. Detach %q first",
					input.Stack, other.Label, policyType, other.Label)), nil, nil
			}
		}
	}

	filePath := input.FormaFile
	if filePath == "" {
		resolved, err := resolveStackFile(cwd, input.Stack, currentEvalFunc())
		if err != nil {
			return errorResult(err), nil, nil
		}
		filePath = resolved
	}

	if err := checkPolicySchemaSupport(filePath); err != nil {
		return errorResult(err), nil, nil
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return errorResult(fmt.Errorf("read %s: %w", filePath, err)), nil, nil
	}

	plan, err := planAttachStandalonePolicy(string(source), input.Stack, input.PolicyLabel, policyType)
	if err != nil {
		return errorResult(err), nil, nil
	}

	out := tools.AttachStandalonePolicyOutput{
		FilePath:             filePath,
		Operation:            plan.Operation,
		PKLSnippet:           plan.PKLSnippet,
		InsertionAnchorStart: plan.AnchorStart,
		InsertionAnchorEnd:   plan.AnchorEnd,
		ImportsToAdd:         plan.ImportsToAdd,
		Notes:                append(notes, plan.Notes...),
	}
	body, err := json.Marshal(out)
	if err != nil {
		return errorResult(fmt.Errorf("marshal output: %w", err)), nil, nil
	}
	return jsonResult(body), nil, nil
}

func (s *Server) handleDetachStandalonePolicy(_ context.Context, _ *mcp.CallToolRequest, input tools.DetachStandalonePolicyInput) (*mcp.CallToolResult, any, error) {
	if input.Stack == "" {
		return errorResult(fmt.Errorf("stack is required")), nil, nil
	}
	if input.PolicyLabel == "" {
		return errorResult(fmt.Errorf("policy_label is required")), nil, nil
	}

	filePath := input.FormaFile
	if filePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errorResult(fmt.Errorf("getwd: %w", err)), nil, nil
		}
		resolved, err := resolveStackFile(cwd, input.Stack, currentEvalFunc())
		if err != nil {
			return errorResult(err), nil, nil
		}
		filePath = resolved
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return errorResult(fmt.Errorf("read %s: %w", filePath, err)), nil, nil
	}

	plan, err := planDetachStandalonePolicy(string(source), input.Stack, input.PolicyLabel)
	if err != nil {
		return errorResult(err), nil, nil
	}

	out := tools.DetachStandalonePolicyOutput{
		FilePath:                  filePath,
		Operation:                 plan.Operation,
		SourceAnchorStart:         plan.AnchorStart,
		SourceAnchorEnd:           plan.AnchorEnd,
		ExistingResolvableSnippet: plan.ExistingSnippet,
		Notes:                     plan.Notes,
	}
	body, err := json.Marshal(out)
	if err != nil {
		return errorResult(fmt.Errorf("marshal output: %w", err)), nil, nil
	}
	return jsonResult(body), nil, nil
}

// specFromInventoryItem converts an agent inventory entry into the spec used to
// render PKL. Reading the config back from the agent (rather than the source)
// means the destroy forma matches what is actually deployed.
func specFromInventoryItem(item policyInventoryItem) (StandalonePolicySpec, error) {
	var cfg policyConfig
	if len(item.Config) > 0 {
		if err := json.Unmarshal(item.Config, &cfg); err != nil {
			return StandalonePolicySpec{}, fmt.Errorf("parse config for policy %q: %w", item.Label, err)
		}
	}
	return StandalonePolicySpec{
		Label:           item.Label,
		PolicyType:      mcpPolicyType(item.Type),
		TTLSeconds:      cfg.TTLSeconds,
		OnDependents:    cfg.OnDependents,
		IntervalSeconds: cfg.IntervalSeconds,
	}, nil
}

func (s *Server) handleDeleteStandalonePolicy(_ context.Context, _ *mcp.CallToolRequest, input tools.DeleteStandalonePolicyInput) (*mcp.CallToolResult, any, error) {
	if input.Label == "" {
		return errorResult(fmt.Errorf("label is required")), nil, nil
	}

	inventory, err := s.fetchPolicies()
	if err != nil {
		return errorResult(err), nil, nil
	}
	item, known := findPolicyByLabel(inventory, input.Label)
	if !known {
		return errorResult(fmt.Errorf(
			"no standalone policy labelled %q is known to the agent; known labels: %v",
			input.Label, policyLabelsOf(inventory))), nil, nil
	}

	// Hard refuse while still attached — destroying an attached policy would be
	// rejected by the agent anyway, and the source edit would already be done.
	if len(item.AttachedStacks) > 0 {
		return errorResult(fmt.Errorf(
			"standalone policy %q is still attached to %d stack(s): %v. "+
				"Detach it from each (detach_standalone_policy) and apply those changes before deleting it",
			input.Label, len(item.AttachedStacks), item.AttachedStacks)), nil, nil
	}

	spec, err := specFromInventoryItem(item)
	if err != nil {
		return errorResult(err), nil, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errorResult(fmt.Errorf("getwd: %w", err)), nil, nil
	}
	filePath, err := resolveStandalonePolicyFile(cwd, input.Label, currentEvalFunc())
	if err != nil {
		return errorResult(err), nil, nil
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return errorResult(fmt.Errorf("read %s: %w", filePath, err)), nil, nil
	}

	plan, err := planDeleteStandalonePolicy(string(source), spec)
	if err != nil {
		return errorResult(err), nil, nil
	}

	out := tools.DeleteStandalonePolicyOutput{
		FilePath:              filePath,
		Operation:             plan.Operation,
		SourceAnchorStart:     plan.AnchorStart,
		SourceAnchorEnd:       plan.AnchorEnd,
		ExistingPolicySnippet: plan.ExistingSnippet,
		DestroyFormaPKL:       renderDestroyFormaPKL(spec),
		Notes:                 plan.Notes,
	}
	body, err := json.Marshal(out)
	if err != nil {
		return errorResult(fmt.Errorf("marshal output: %w", err)), nil, nil
	}
	return jsonResult(body), nil, nil
}
