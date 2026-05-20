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
