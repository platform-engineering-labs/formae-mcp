package server

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// StandalonePolicyPlan is the planned edit returned by every standalone-policy
// planner. Anchors are 1-indexed inclusive line numbers. For an insertion
// (AnchorStart == AnchorEnd) the snippet goes BEFORE that line; for a
// replacement or deletion the range is replaced or removed.
//
// The tool never writes the file — the caller applies the plan with Edit.
type StandalonePolicyPlan struct {
	Operation       string
	PKLSnippet      string
	ExistingSnippet string
	AnchorStart     int
	AnchorEnd       int
	ImportsToAdd    []string
	Notes           []string
}

// missingImports returns the formae import if the source lacks it.
func missingImports(source string) []string {
	if strings.Contains(source, formaeImport) {
		return []string{}
	}
	return []string{formaeImport}
}

// planCreateStandalonePolicy plans the insertion of a new standalone policy
// declaration just before the closing brace of the top-level forma block.
// Returns a noop when a standalone with that label already exists — updating a
// standalone in place is out of scope for v2 (delete + recreate instead).
func planCreateStandalonePolicy(source string, spec StandalonePolicySpec) (StandalonePolicyPlan, error) {
	_, formaEnd, ok := findFormaBlock(source)
	if !ok {
		return StandalonePolicyPlan{}, fmt.Errorf(
			"no `forma { ... }` block found in the target file; a standalone policy must be declared inside one")
	}

	if decl, exists := findStandalonePolicyDeclaration(source, spec.Label); exists {
		return StandalonePolicyPlan{
			Operation: "noop",
			Notes: []string{fmt.Sprintf(
				"a standalone policy labelled %q is already declared at lines %d-%d; "+
					"updating a standalone in place is not supported — delete it and recreate it",
				spec.Label, decl.StartLine, decl.EndLine)},
		}, nil
	}

	snippet := renderStandalonePolicyPKL(spec)
	if snippet == "" {
		return StandalonePolicyPlan{}, fmt.Errorf(
			"unknown policy_type %q (must be 'ttl' or 'auto_reconcile')", spec.PolicyType)
	}

	return StandalonePolicyPlan{
		Operation:    "create",
		PKLSnippet:   snippet,
		AnchorStart:  formaEnd,
		AnchorEnd:    formaEnd,
		ImportsToAdd: missingImports(source),
	}, nil
}

// planAttachStandalonePolicy plans the insertion of a PolicyResolvable entry
// into a stack's policies listing. policyType is the MCP form ("ttl" |
// "auto_reconcile") and is used only for the inline-conflict check.
//
// Returns a noop when the policy is already attached (either form). Returns an
// error when the stack is absent from the source or already carries an inline
// policy of the same type — a stack may not hold both.
func planAttachStandalonePolicy(source, stackLabel, policyLabel, policyType string) (StandalonePolicyPlan, error) {
	_, stackEnd, ok := findStackBlock(source, stackLabel)
	if !ok {
		return StandalonePolicyPlan{}, fmt.Errorf("stack %q not found in source", stackLabel)
	}

	if start, end, attached := findResolvableInPoliciesBlock(source, stackLabel, policyLabel); attached {
		return StandalonePolicyPlan{
			Operation: "noop",
			Notes: []string{fmt.Sprintf(
				"standalone policy %q is already attached to stack %q (lines %d-%d); nothing to do",
				policyLabel, stackLabel, start, end)},
		}, nil
	}

	if inlineStart, inlineEnd, hasInline := findExistingPolicy(source, stackLabel, policyType); hasInline {
		return StandalonePolicyPlan{}, fmt.Errorf(
			"stack %q already has an inline %s policy at lines %d-%d; a stack cannot hold both an inline "+
				"and a standalone policy of the same type. Remove the inline policy first "+
				"(create_inline_policy with operation=remove), then attach",
			stackLabel, policyType, inlineStart, inlineEnd)
	}

	entry := renderPolicyResolvablePKL(policyLabel)

	if innerStart, innerEnd, hasBlock := findPoliciesBlock(source, stackLabel); hasBlock {
		closingLine := innerEnd + 1
		if closingLine <= 0 {
			closingLine = innerStart
		}
		return StandalonePolicyPlan{
			Operation:    "attach",
			PKLSnippet:   entry,
			AnchorStart:  closingLine,
			AnchorEnd:    closingLine,
			ImportsToAdd: missingImports(source),
		}, nil
	}

	return StandalonePolicyPlan{
		Operation:    "attach",
		PKLSnippet:   wrapInPoliciesListing(entry),
		AnchorStart:  stackEnd,
		AnchorEnd:    stackEnd,
		ImportsToAdd: missingImports(source),
	}, nil
}

// planDetachStandalonePolicy plans the removal of a PolicyResolvable entry from
// a stack's policies listing. When the entry is the listing's only member the
// anchor covers the whole `policies = new Listing { ... }` wrapper, mirroring
// create_inline_policy's remove behaviour.
//
// Returns a noop when the policy is not attached to this stack.
func planDetachStandalonePolicy(source, stackLabel, policyLabel string) (StandalonePolicyPlan, error) {
	if _, _, ok := findStackBlock(source, stackLabel); !ok {
		return StandalonePolicyPlan{}, fmt.Errorf("stack %q not found in source", stackLabel)
	}

	entryStart, entryEnd, attached := findResolvableInPoliciesBlock(source, stackLabel, policyLabel)
	if !attached {
		return StandalonePolicyPlan{
			Operation: "noop",
			Notes: []string{fmt.Sprintf(
				"standalone policy %q is not attached to stack %q in this file; nothing to detach",
				policyLabel, stackLabel)},
		}, nil
	}

	existing := extractLines(source, entryStart, entryEnd)

	// countPoliciesInBlock counts inline blocks, direct resolvables and .res
	// bindings alike, so a count of 1 genuinely means this entry is the only one.
	count, _ := countPoliciesInBlock(source, stackLabel)
	if innerStart, innerEnd, hasBlock := findPoliciesBlock(source, stackLabel); hasBlock && count == 1 {
		return StandalonePolicyPlan{
			Operation:       "detach",
			ExistingSnippet: existing,
			AnchorStart:     innerStart - 1, // the `policies = new Listing {` line
			AnchorEnd:       innerEnd + 1,   // its closing `}`
			Notes:           []string{"removed empty policies block (was the only policy)"},
		}, nil
	}

	return StandalonePolicyPlan{
		Operation:       "detach",
		ExistingSnippet: existing,
		AnchorStart:     entryStart,
		AnchorEnd:       entryEnd,
	}, nil
}

// renderDestroyFormaPKL emits a complete, self-contained forma file declaring
// only the given policy. The skill writes this to a temp file and passes it to
// destroy_forma — removing the declaration from source is not enough on its
// own, because the agent only forgets a policy when it is destroyed.
func renderDestroyFormaPKL(spec StandalonePolicySpec) string {
	return fmt.Sprintf(`amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
%s
}
`, indentLines(renderStandalonePolicyPKL(spec), "  "))
}

// indentLines prefixes every line of s with indent.
func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// planDeleteStandalonePolicy locates a standalone policy's declaration in the
// source and plans its removal. spec carries the policy's config as the AGENT
// holds it, so the destroy forma matches deployed state rather than whatever
// the source happens to say.
func planDeleteStandalonePolicy(source string, spec StandalonePolicySpec) (StandalonePolicyPlan, error) {
	decl, ok := findStandalonePolicyDeclaration(source, spec.Label)
	if !ok {
		return StandalonePolicyPlan{}, fmt.Errorf(
			"standalone policy %q is not declared in this file; the agent knows it but the source "+
				"declaration could not be located", spec.Label)
	}

	notes := []string{}
	if decl.LocalBinding != "" {
		notes = append(notes, fmt.Sprintf(
			"this declaration is bound to local %q — after deleting these lines you must also remove the "+
				"bare %q reference inside forma { } and any %q entries in stack policies listings, "+
				"or the file will not evaluate",
			decl.LocalBinding, decl.LocalBinding, decl.LocalBinding+".res"))
	}

	return StandalonePolicyPlan{
		Operation:       "delete",
		ExistingSnippet: extractLines(source, decl.StartLine, decl.EndLine),
		AnchorStart:     decl.StartLine,
		AnchorEnd:       decl.EndLine,
		Notes:           notes,
	}, nil
}

// standalonePolicyTypeFromWorkspace finds a standalone policy's declaration in
// the workspace and reports its MCP policy type. Used when the agent does not
// know the policy yet — a declaration written to source but not yet applied is
// legitimately absent from the agent inventory.
//
// Returns found=false when no file declares it. Propagates the ambiguity error
// when several files do, since that is a real problem the user must resolve.
func standalonePolicyTypeFromWorkspace(root, label string, eval EvalFunc) (string, bool, error) {
	path, err := resolveStandalonePolicyFile(root, label, eval)
	if err != nil {
		var notFound *policySourceNotFoundError
		if errors.As(err, &notFound) {
			return "", false, nil
		}
		return "", false, err
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	decl, ok := findStandalonePolicyDeclaration(string(source), label)
	if !ok || decl.PolicyType == "" {
		return "", false, nil
	}
	return decl.PolicyType, true, nil
}
