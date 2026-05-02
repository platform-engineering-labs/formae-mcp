package server

import (
	"fmt"
	"regexp"
	"strings"
)

// PolicyEditPlan describes a planned edit to a stack's inline policies.
// The plan is informational — the caller (skill / LLM) applies the snippet
// at the returned line range using the Edit tool. The tool does not modify
// the file directly.
type PolicyEditPlan struct {
	Operation             string
	PKLSnippet            string
	InsertionAnchorStart  int
	InsertionAnchorEnd    int
	ExistingPolicySnippet string
	ImportsToAdd          []string
	Notes                 []string
}

// PolicySpec describes the desired policy state for an edit.
type PolicySpec struct {
	StackLabel      string
	PolicyType      string // "ttl" | "auto_reconcile"
	Operation       string // "set" | "remove"
	TTLSeconds      int64
	OnDependents    string // "abort" | "cascade", default "abort"
	IntervalSeconds int64
}

const formaeImport = `import "@formae/formae.pkl"`

var (
	stackBlockStartRE  = regexp.MustCompile(`new\s+formae\.Stack\s*\{`)
	stackLabelRE       = regexp.MustCompile(`label\s*=\s*"([^"]+)"`)
	policiesAssignRE   = regexp.MustCompile(`policies\s*=\s*new(?:\s+\w+)?\s*\{`)
	anyPolicyEntryRE   = regexp.MustCompile(`new\s+formae\.\w+Policy\s*\{`)
	policyTypeClassMap = map[string]string{
		"ttl":            "TTLPolicy",
		"auto_reconcile": "AutoReconcilePolicy",
	}
)

// renderTTLPolicyPKL returns a PKL snippet for a TTL policy entry (no policies wrapper).
func renderTTLPolicyPKL(ttlSeconds int64, onDependents string) string {
	return fmt.Sprintf(`new formae.TTLPolicy {
  ttl = %s
  onDependents = "%s"
}`, formatPKLDuration(ttlSeconds), onDependents)
}

// renderAutoReconcilePolicyPKL returns a PKL snippet for an auto-reconcile policy entry.
func renderAutoReconcilePolicyPKL(intervalSeconds int64) string {
	return fmt.Sprintf(`new formae.AutoReconcilePolicy {
  interval = %s
}`, formatPKLDuration(intervalSeconds))
}

// formatPKLDuration converts a duration in seconds to a PKL Duration literal,
// picking the largest unit that yields a clean integer.
func formatPKLDuration(seconds int64) string {
	switch {
	case seconds > 0 && seconds%86400 == 0:
		return fmt.Sprintf("%d.d", seconds/86400)
	case seconds > 0 && seconds%3600 == 0:
		return fmt.Sprintf("%d.h", seconds/3600)
	case seconds > 0 && seconds%60 == 0:
		return fmt.Sprintf("%d.min", seconds/60)
	default:
		return fmt.Sprintf("%d.s", seconds)
	}
}

// findStackBlock locates a `new formae.Stack { ... }` block whose label matches.
// Returns 1-indexed inclusive line range and ok=true on success. Brace depth
// is tracked so nested blocks (policies, etc.) don't confuse the matcher.
func findStackBlock(source, label string) (int, int, bool) {
	starts := stackBlockStartRE.FindAllStringIndex(source, -1)
	for _, m := range starts {
		startIdx := m[0]
		openIdx := strings.Index(source[startIdx:], "{") + startIdx
		endIdx, ok := matchBrace(source, openIdx)
		if !ok {
			continue
		}
		body := source[openIdx+1 : endIdx]
		labelMatch := stackLabelRE.FindStringSubmatch(body)
		if labelMatch == nil || labelMatch[1] != label {
			continue
		}
		return lineNumber(source, startIdx), lineNumber(source, endIdx), true
	}
	return 0, 0, false
}

// findPoliciesBlock locates the `policies = new Listing { ... }` block inside
// the named stack. Returns the *inner* line range (lines between `{` and `}`,
// excluding the brace lines themselves). Returns (0, 0, false) if there is no
// policies block on the stack.
func findPoliciesBlock(source, stackLabel string) (int, int, bool) {
	stackStart, stackEnd, ok := findStackBlock(source, stackLabel)
	if !ok {
		return 0, 0, false
	}
	stackStartOffset := offsetOfLine(source, stackStart)
	stackEndOffset := offsetOfLine(source, stackEnd+1)
	if stackEndOffset > len(source) {
		stackEndOffset = len(source)
	}
	stackBody := source[stackStartOffset:stackEndOffset]

	loc := policiesAssignRE.FindStringIndex(stackBody)
	if loc == nil {
		return 0, 0, false
	}
	openIdx := strings.Index(stackBody[loc[0]:], "{") + loc[0]
	closeIdx, ok := matchBrace(stackBody, openIdx)
	if !ok {
		return 0, 0, false
	}
	openLineInBody := lineNumber(stackBody, openIdx)
	closeLineInBody := lineNumber(stackBody, closeIdx)
	innerStart := stackStart + openLineInBody
	innerEnd := stackStart + closeLineInBody - 2
	if innerEnd < innerStart {
		// Empty or single-line policies block — collapse to the line just
		// before the closing brace.
		innerStart = stackStart + closeLineInBody - 2
		innerEnd = innerStart
	}
	return innerStart, innerEnd, true
}

// findExistingPolicy locates a `new formae.<ClassName> { ... }` block of the
// given type inside the named stack's policies block. Returns the 1-indexed
// inclusive line range covering the policy's `new formae.X {` opening through
// its closing `}`.
func findExistingPolicy(source, stackLabel, policyType string) (int, int, bool) {
	className, known := policyTypeClassMap[policyType]
	if !known {
		return 0, 0, false
	}
	innerStart, innerEnd, ok := findPoliciesBlock(source, stackLabel)
	if !ok {
		return 0, 0, false
	}
	startOffset := offsetOfLine(source, innerStart)
	endOffset := offsetOfLine(source, innerEnd+1)
	if endOffset > len(source) {
		endOffset = len(source)
	}
	body := source[startOffset:endOffset]

	pattern := regexp.MustCompile(`new\s+formae\.` + regexp.QuoteMeta(className) + `\s*\{`)
	loc := pattern.FindStringIndex(body)
	if loc == nil {
		return 0, 0, false
	}
	openIdx := strings.Index(body[loc[0]:], "{") + loc[0]
	closeIdx, ok := matchBrace(body, openIdx)
	if !ok {
		return 0, 0, false
	}
	startLineInBody := lineNumber(body, loc[0])
	endLineInBody := lineNumber(body, closeIdx)
	return innerStart + startLineInBody - 1, innerStart + endLineInBody - 1, true
}

// planPolicyEdit ties everything together: resolves the stack, computes the
// snippet, and returns a PolicyEditPlan. Returns an error if the stack is not
// found in the source.
func planPolicyEdit(source string, spec PolicySpec) (PolicyEditPlan, error) {
	stackStart, stackEnd, ok := findStackBlock(source, spec.StackLabel)
	if !ok {
		return PolicyEditPlan{}, fmt.Errorf("stack %q not found in source", spec.StackLabel)
	}
	_ = stackStart

	imports := []string{}
	if !strings.Contains(source, formaeImport) {
		imports = append(imports, formaeImport)
	}

	existingStart, existingEnd, hasExisting := findExistingPolicy(source, spec.StackLabel, spec.PolicyType)
	innerStart, innerEnd, hasPoliciesBlock := findPoliciesBlock(source, spec.StackLabel)

	switch spec.Operation {
	case "set":
		entry := renderPolicyEntry(spec)
		if hasExisting {
			return PolicyEditPlan{
				Operation:             "update",
				PKLSnippet:            entry,
				InsertionAnchorStart:  existingStart,
				InsertionAnchorEnd:    existingEnd,
				ExistingPolicySnippet: extractLines(source, existingStart, existingEnd),
				ImportsToAdd:          imports,
			}, nil
		}
		if hasPoliciesBlock {
			closingLine := innerEnd + 1
			if closingLine <= 0 {
				closingLine = innerStart
			}
			return PolicyEditPlan{
				Operation:            "create",
				PKLSnippet:           entry,
				InsertionAnchorStart: closingLine,
				InsertionAnchorEnd:   closingLine,
				ImportsToAdd:         imports,
			}, nil
		}
		return PolicyEditPlan{
			Operation:            "create",
			PKLSnippet:           wrapInPoliciesListing(entry),
			InsertionAnchorStart: stackEnd,
			InsertionAnchorEnd:   stackEnd,
			ImportsToAdd:         imports,
		}, nil

	case "remove":
		if !hasExisting {
			return PolicyEditPlan{
				Operation: "noop",
				Notes:     []string{fmt.Sprintf("no %s policy was attached to stack %q; nothing to remove", spec.PolicyType, spec.StackLabel)},
			}, nil
		}
		policyCount, _ := countPoliciesInBlock(source, spec.StackLabel)
		if policyCount == 1 && hasPoliciesBlock {
			policiesStartLine := innerStart - 1
			policiesEndLine := innerEnd + 1
			return PolicyEditPlan{
				Operation:             "remove",
				InsertionAnchorStart:  policiesStartLine,
				InsertionAnchorEnd:    policiesEndLine,
				ExistingPolicySnippet: extractLines(source, existingStart, existingEnd),
				Notes:                 []string{"removed empty policies block (was the only policy)"},
			}, nil
		}
		return PolicyEditPlan{
			Operation:             "remove",
			InsertionAnchorStart:  existingStart,
			InsertionAnchorEnd:    existingEnd,
			ExistingPolicySnippet: extractLines(source, existingStart, existingEnd),
		}, nil

	default:
		return PolicyEditPlan{}, fmt.Errorf("unknown operation %q (must be set or remove)", spec.Operation)
	}
}

func renderPolicyEntry(spec PolicySpec) string {
	switch spec.PolicyType {
	case "ttl":
		dep := spec.OnDependents
		if dep == "" {
			dep = "abort"
		}
		return renderTTLPolicyPKL(spec.TTLSeconds, dep)
	case "auto_reconcile":
		return renderAutoReconcilePolicyPKL(spec.IntervalSeconds)
	default:
		return ""
	}
}

func wrapInPoliciesListing(entry string) string {
	indented := strings.ReplaceAll(entry, "\n", "\n  ")
	return "policies = new Listing {\n  " + indented + "\n}"
}

func countPoliciesInBlock(source, stackLabel string) (int, bool) {
	innerStart, innerEnd, ok := findPoliciesBlock(source, stackLabel)
	if !ok {
		return 0, false
	}
	startOffset := offsetOfLine(source, innerStart)
	endOffset := offsetOfLine(source, innerEnd+1)
	if endOffset > len(source) {
		endOffset = len(source)
	}
	body := source[startOffset:endOffset]
	return len(anyPolicyEntryRE.FindAllString(body, -1)), true
}

// matchBrace returns the index of the closing `}` matching the `{` at openIdx.
func matchBrace(source string, openIdx int) (int, bool) {
	if openIdx >= len(source) || source[openIdx] != '{' {
		return 0, false
	}
	depth := 0
	for i := openIdx; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// lineNumber returns the 1-indexed line containing offset.
func lineNumber(source string, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	return strings.Count(source[:offset], "\n") + 1
}

// offsetOfLine returns the byte offset of the start of a 1-indexed line.
func offsetOfLine(source string, line int) int {
	if line <= 1 {
		return 0
	}
	count := 1
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			count++
			if count == line {
				return i + 1
			}
		}
	}
	return len(source)
}

// extractLines returns the substring covering lines [start, end] (1-indexed, inclusive).
func extractLines(source string, start, end int) string {
	startOffset := offsetOfLine(source, start)
	endOffset := offsetOfLine(source, end+1)
	if endOffset > len(source) {
		endOffset = len(source)
	}
	if startOffset > endOffset {
		return ""
	}
	return source[startOffset:endOffset]
}
