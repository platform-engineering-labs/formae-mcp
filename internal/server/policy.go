package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

	// A stack may hold only one policy per type. If a standalone of this type is
	// already attached, setting an inline one would conflict — refuse up front so
	// the rule holds even if a skill author forgets it. Removal is exempt:
	// deleting an inline policy can never create a conflict, and the check would
	// make `remove` fail whenever the agent is unreachable.
	//
	// An unreachable agent is not fatal here either: this tool's real work is
	// local file planning, so a transport failure downgrades to "no standalone
	// policies known" rather than blocking the edit.
	if input.Operation == "set" {
		if inventory, err := s.fetchPolicies(); err == nil {
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

	if err := checkPolicySchemaSupport(filePath); err != nil {
		return errorResult(err), nil, nil
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return errorResult(fmt.Errorf("read %s: %w", filePath, err)), nil, nil
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
