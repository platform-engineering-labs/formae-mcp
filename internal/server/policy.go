package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// injectedEvalForTest allows tests to substitute a fake EvalFunc without
// depending on the formae binary. Production code uses formaeEval.
var injectedEvalForTest EvalFunc

func currentEvalFunc() EvalFunc {
	if injectedEvalForTest != nil {
		return injectedEvalForTest
	}
	return formaeEval
}

func (s *Server) handleCreateInlinePolicy(_ context.Context, _ *mcp.CallToolRequest, input tools.CreateInlinePolicyInput) (*mcp.CallToolResult, any, error) {
	if err := validateCreateInlinePolicyInput(input); err != nil {
		return errorResult(err), nil, nil
	}

	// Setting an inline auto-reconcile policy requires the agent-side fix that
	// shipped in formae 0.88.0 (before it, the label was dropped and the policy
	// churned a phantom update every apply). TTL and removals are unaffected.
	if input.Operation == "set" && input.PolicyType == "auto_reconcile" {
		if err := featuregate.GuardFeature(featuregate.FeatureAutoReconcilePolicy); err != nil {
			return errorResult(err), nil, nil
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errorResult(fmt.Errorf("getwd: %w", err)), nil, nil
	}

	// A stack may hold only one policy per type. Setting an inline policy on a
	// stack that already carries a standalone of the same type — whether applied
	// (agent inventory) or only in source (attached but not yet applied) — is a
	// conflict. Removal is exempt: deleting an inline policy can never create a
	// conflict. An unreachable agent is not fatal; this tool's real work is
	// local file planning, so a transport failure downgrades to "no standalone
	// policies known from the agent" and the source check still runs.
	var inventory []policyInventoryItem
	if input.Operation == "set" {
		if items, err := s.fetchPolicies(); err == nil {
			inventory = items
		}
		for _, item := range inventory {
			if mcpPolicyType(item.Type) != input.PolicyType {
				continue
			}
			for _, attached := range item.AttachedStacks {
				if attached != input.Stack {
					continue
				}
				return errorResult(fmt.Errorf(
					"stack %q already has standalone policy %q of type %s attached; a stack cannot hold "+
						"both an inline and a standalone policy of the same type. Detach %q first "+
						"(detach_standalone_policy), or update the standalone instead",
					input.Stack, item.Label, input.PolicyType, item.Label)), nil, nil
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

	// Source-only mirror of the agent check above: a same-type standalone
	// attached to this stack in source but not yet applied.
	if input.Operation == "set" {
		for _, lbl := range resolvableLabelsInPoliciesBlock(string(source), input.Stack) {
			if t, ok := s.standaloneTypeOf(lbl, inventory, cwd); ok && t == input.PolicyType {
				return errorResult(fmt.Errorf(
					"stack %q already has standalone policy %q of type %s attached in source; a stack cannot "+
						"hold both an inline and a standalone policy of the same type. Detach %q first "+
						"(detach_standalone_policy)",
					input.Stack, lbl, input.PolicyType, lbl)), nil, nil
			}
		}
	}

	plan, err := planPolicyEdit(string(source), policySpecFromInput(input))
	if err != nil {
		return errorResult(err), nil, nil
	}

	out := tools.CreateInlinePolicyOutput{
		FilePath:              filePath,
		Operation:             plan.Operation,
		PKLSnippet:            plan.PKLSnippet,
		InsertionAnchorStart:  plan.InsertionAnchorStart,
		InsertionAnchorEnd:    plan.InsertionAnchorEnd,
		ExistingPolicySnippet: plan.ExistingPolicySnippet,
		ImportsToAdd:          plan.ImportsToAdd,
		Notes:                 plan.Notes,
	}
	body, err := json.Marshal(out)
	if err != nil {
		return errorResult(fmt.Errorf("marshal output: %w", err)), nil, nil
	}
	return jsonResult(body), nil, nil
}

func validateCreateInlinePolicyInput(input tools.CreateInlinePolicyInput) error {
	if input.Stack == "" {
		return fmt.Errorf("stack is required")
	}
	switch input.PolicyType {
	case "ttl", "auto_reconcile":
	default:
		return fmt.Errorf("policy_type must be 'ttl' or 'auto_reconcile', got %q", input.PolicyType)
	}
	switch input.Operation {
	case "set", "remove":
	default:
		return fmt.Errorf("operation must be 'set' or 'remove', got %q", input.Operation)
	}
	if input.Operation == "set" {
		switch input.PolicyType {
		case "ttl":
			if input.TTLSeconds <= 0 {
				return fmt.Errorf("ttl_seconds is required and must be > 0 when policy_type=ttl operation=set")
			}
			if input.OnDependents != "" && input.OnDependents != "abort" && input.OnDependents != "cascade" {
				return fmt.Errorf("on_dependents must be 'abort' or 'cascade', got %q", input.OnDependents)
			}
		case "auto_reconcile":
			if input.IntervalSeconds <= 0 {
				return fmt.Errorf("interval_seconds is required and must be > 0 when policy_type=auto_reconcile operation=set")
			}
		}
	}
	return nil
}

func policySpecFromInput(input tools.CreateInlinePolicyInput) PolicySpec {
	dep := input.OnDependents
	if dep == "" {
		dep = "abort"
	}
	return PolicySpec{
		StackLabel:      input.Stack,
		PolicyType:      input.PolicyType,
		Operation:       input.Operation,
		TTLSeconds:      input.TTLSeconds,
		OnDependents:    dep,
		IntervalSeconds: input.IntervalSeconds,
	}
}
