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
	stackBlockStartRE = regexp.MustCompile(`new\s+formae\.Stack\s*\{`)
	stackLabelRE      = regexp.MustCompile(`label\s*=\s*"([^"]+)"`)
	policiesAssignRE  = regexp.MustCompile(`policies\s*=\s*new(?:\s+\w+)?\s*\{`)
	// A policies listing can hold three kinds of entry:
	//   1. an inline policy block:  new formae.TTLPolicy { ... }
	//   2. a direct resolvable:     new formae.PolicyResolvable { label = "X" }
	//   3. a binding resolvable:    ephemeral.res
	// anyPolicyEntryRE deliberately does NOT match PolicyResolvable — the class
	// name does not end in "Policy" — so the three patterns never double-count.
	anyPolicyEntryRE        = regexp.MustCompile(`new\s+formae\.\w+Policy\s*\{`)
	policyResolvableEntryRE = regexp.MustCompile(`new\s+formae\.PolicyResolvable\s*\{`)
	policyResBindingRE      = regexp.MustCompile(`(?m)^\s*[A-Za-z_]\w*\.res\s*$`)
	// formaBlockStartRE matches the top-level `forma {` opener. The `\s*\{`
	// after `forma` is what keeps `new formae.Stack {` from matching — in that
	// string the character after "forma" is "e".
	formaBlockStartRE = regexp.MustCompile(`(?m)^\s*forma\s*\{`)
	// localBindingRE captures the binding name in `local <name> = ` immediately
	// preceding a policy declaration.
	localBindingRE = regexp.MustCompile(`local\s+([A-Za-z_]\w*)\s*=\s*$`)
	// policyResBindingCaptureRE is policyResBindingRE with the binding name
	// captured, for resolving `<binding>.res` back to a policy label.
	policyResBindingCaptureRE = regexp.MustCompile(`(?m)^\s*([A-Za-z_]\w*)\.res\s*$`)
	policyTypeClassMap        = map[string]string{
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

// countPoliciesInBlock counts every entry inside the stack's policies listing:
// inline policy blocks, direct `new formae.PolicyResolvable { ... }` entries,
// and `<binding>.res` entries. Returns 0, false if there is no policies block.
// Correctness matters — planPolicyEdit uses a count of 1 to decide it may
// delete the whole `policies = new Listing { ... }` wrapper.
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
	count := len(anyPolicyEntryRE.FindAllString(body, -1)) +
		len(policyResolvableEntryRE.FindAllString(body, -1)) +
		len(policyResBindingRE.FindAllString(body, -1))
	return count, true
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

// StandalonePolicySpec describes a reusable policy declared at the top level of
// a forma block. PolicyType uses the MCP wire vocabulary ("ttl" |
// "auto_reconcile"), not the agent's ("ttl" | "auto-reconcile").
type StandalonePolicySpec struct {
	Label           string
	PolicyType      string
	TTLSeconds      int64
	OnDependents    string
	IntervalSeconds int64
}

// renderStandalonePolicyPKL emits a complete standalone policy declaration,
// including the label (which inline policies may omit but standalone policies
// must carry — the agent rejects an unlabelled standalone policy).
func renderStandalonePolicyPKL(spec StandalonePolicySpec) string {
	switch spec.PolicyType {
	case "ttl":
		onDependents := spec.OnDependents
		if onDependents == "" {
			onDependents = "abort"
		}
		return fmt.Sprintf(`new formae.TTLPolicy {
  label = %q
  ttl = %s
  onDependents = %q
}`, spec.Label, formatPKLDuration(spec.TTLSeconds), onDependents)
	case "auto_reconcile":
		return fmt.Sprintf(`new formae.AutoReconcilePolicy {
  label = %q
  interval = %s
}`, spec.Label, formatPKLDuration(spec.IntervalSeconds))
	default:
		// Unknown type — callers treat "" as "reject this spec" rather than
		// silently emitting a policy of the wrong kind.
		return ""
	}
}

// renderPolicyResolvablePKL emits the attachment entry that references a
// standalone policy by label. This is the direct form; the equivalent
// `<binding>.res` form renders to the same $ref but needs a local binding,
// so it is read but never written.
func renderPolicyResolvablePKL(label string) string {
	return fmt.Sprintf(`new formae.PolicyResolvable {
  label = %q
}`, label)
}

// findFormaBlock returns the 1-indexed inclusive line range of the top-level
// `forma { ... }` block. Standalone policies are declared inside it.
func findFormaBlock(source string) (int, int, bool) {
	loc := formaBlockStartRE.FindStringIndex(source)
	if loc == nil {
		return 0, 0, false
	}
	openIdx := strings.Index(source[loc[0]:loc[1]], "{") + loc[0]
	closeIdx, ok := matchBrace(source, openIdx)
	if !ok {
		return 0, 0, false
	}
	return lineNumber(source, openIdx), lineNumber(source, closeIdx), true
}

// standaloneDeclaration is the located source of a standalone policy.
// LocalBinding is non-empty when the declaration is bound to a `local` name,
// in which case deleting the declaration alone leaves a dangling reference —
// callers must surface that to the user.
type standaloneDeclaration struct {
	StartLine int
	EndLine   int
	// PolicyType is the MCP wire form ("ttl" | "auto_reconcile"), derived from
	// the declared PKL class. Lets callers learn a policy's type from source
	// when the agent does not know it yet (declared but not yet applied).
	PolicyType   string
	LocalBinding string
}

// policyClassNameRE captures the class name of a policy declaration, e.g.
// "TTLPolicy" from `new formae.TTLPolicy {`.
var policyClassNameRE = regexp.MustCompile(`new\s+formae\.(\w+Policy)\s*\{`)

// mcpTypeForPolicyClass reverses policyTypeClassMap: PKL class name -> MCP
// wire type. Returns "" for an unrecognised class.
func mcpTypeForPolicyClass(className string) string {
	for mcpType, class := range policyTypeClassMap {
		if class == className {
			return mcpType
		}
	}
	return ""
}

// allStackBlockRanges returns the byte-offset [start, end] range of every
// `new formae.Stack { ... }` block in the source.
func allStackBlockRanges(source string) [][2]int {
	var ranges [][2]int
	for _, m := range stackBlockStartRE.FindAllStringIndex(source, -1) {
		openIdx := strings.Index(source[m[0]:], "{") + m[0]
		closeIdx, ok := matchBrace(source, openIdx)
		if !ok {
			continue
		}
		ranges = append(ranges, [2]int{m[0], closeIdx})
	}
	return ranges
}

// findStandalonePolicyDeclaration locates the declaration of a standalone
// policy by label. A declaration qualifies when its block carries the matching
// `label = "..."` and it does not sit inside any Stack block — the latter is
// what distinguishes a standalone from a labelled inline policy.
//
// Both shapes are recognised: a direct `new formae.<X>Policy { ... }` inside
// forma { }, and a `local <name> = new formae.<X>Policy { ... }` outside it.
func findStandalonePolicyDeclaration(source, label string) (standaloneDeclaration, bool) {
	stackRanges := allStackBlockRanges(source)

	for _, m := range anyPolicyEntryRE.FindAllStringIndex(source, -1) {
		startIdx := m[0]

		inStack := false
		for _, r := range stackRanges {
			if startIdx > r[0] && startIdx < r[1] {
				inStack = true
				break
			}
		}
		if inStack {
			continue
		}

		openIdx := strings.Index(source[startIdx:], "{") + startIdx
		closeIdx, ok := matchBrace(source, openIdx)
		if !ok {
			continue
		}
		body := source[openIdx+1 : closeIdx]
		labelMatch := stackLabelRE.FindStringSubmatch(body)
		if labelMatch == nil || labelMatch[1] != label {
			continue
		}

		decl := standaloneDeclaration{
			StartLine: lineNumber(source, startIdx),
			EndLine:   lineNumber(source, closeIdx),
		}
		if classMatch := policyClassNameRE.FindStringSubmatch(source[startIdx : openIdx+1]); classMatch != nil {
			decl.PolicyType = mcpTypeForPolicyClass(classMatch[1])
		}
		// Look backwards on the same line for a `local <name> = ` prefix.
		lineStart := offsetOfLine(source, decl.StartLine)
		if bindingMatch := localBindingRE.FindStringSubmatch(source[lineStart:startIdx]); bindingMatch != nil {
			decl.LocalBinding = bindingMatch[1]
		}
		return decl, true
	}
	return standaloneDeclaration{}, false
}

// policyLabelForBinding resolves a `local <binding> = new formae.<X>Policy
// { ... label = "L" ... }` declaration back to L. Returns ok=false when the
// binding is not declared in this file — e.g. it arrives via an import. Callers
// treat that as "no match" rather than guessing.
func policyLabelForBinding(source, binding string) (string, bool) {
	pattern := regexp.MustCompile(`local\s+` + regexp.QuoteMeta(binding) + `\s*=\s*new\s+formae\.\w+Policy\s*\{`)
	loc := pattern.FindStringIndex(source)
	if loc == nil {
		return "", false
	}
	openIdx := strings.Index(source[loc[0]:], "{") + loc[0]
	closeIdx, ok := matchBrace(source, openIdx)
	if !ok {
		return "", false
	}
	labelMatch := stackLabelRE.FindStringSubmatch(source[openIdx+1 : closeIdx])
	if labelMatch == nil {
		return "", false
	}
	return labelMatch[1], true
}

// findResolvableInPoliciesBlock locates the entry attaching the named
// standalone policy to the named stack. Recognises both the direct
// `new formae.PolicyResolvable { label = "L" }` form and the `<binding>.res`
// form. Returns the 1-indexed inclusive line range of the entry.
func findResolvableInPoliciesBlock(source, stackLabel, policyLabel string) (int, int, bool) {
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

	// Direct form.
	for _, m := range policyResolvableEntryRE.FindAllStringIndex(body, -1) {
		openIdx := strings.Index(body[m[0]:], "{") + m[0]
		closeIdx, matched := matchBrace(body, openIdx)
		if !matched {
			continue
		}
		labelMatch := stackLabelRE.FindStringSubmatch(body[openIdx+1 : closeIdx])
		if labelMatch == nil || labelMatch[1] != policyLabel {
			continue
		}
		return innerStart + lineNumber(body, m[0]) - 1,
			innerStart + lineNumber(body, closeIdx) - 1,
			true
	}

	// `<binding>.res` form — resolve the binding against the whole file.
	for _, m := range policyResBindingCaptureRE.FindAllStringSubmatchIndex(body, -1) {
		binding := body[m[2]:m[3]]
		label, resolved := policyLabelForBinding(source, binding)
		if !resolved || label != policyLabel {
			continue
		}
		line := innerStart + lineNumber(body, m[0]) - 1
		return line, line, true
	}

	return 0, 0, false
}

// resolvableLabelsInPoliciesBlock returns the labels of every standalone policy
// attached to the named stack in source — both direct
// `new formae.PolicyResolvable { label = "X" }` entries and `<binding>.res`
// references (resolved against the file). Used to detect same-type conflicts
// that live only in source, before the first apply makes them visible to the
// agent.
func resolvableLabelsInPoliciesBlock(source, stackLabel string) []string {
	innerStart, innerEnd, ok := findPoliciesBlock(source, stackLabel)
	if !ok {
		return nil
	}
	startOffset := offsetOfLine(source, innerStart)
	endOffset := offsetOfLine(source, innerEnd+1)
	if endOffset > len(source) {
		endOffset = len(source)
	}
	body := source[startOffset:endOffset]

	var labels []string
	for _, m := range policyResolvableEntryRE.FindAllStringIndex(body, -1) {
		openIdx := strings.Index(body[m[0]:], "{") + m[0]
		closeIdx, matched := matchBrace(body, openIdx)
		if !matched {
			continue
		}
		if lm := stackLabelRE.FindStringSubmatch(body[openIdx+1 : closeIdx]); lm != nil {
			labels = append(labels, lm[1])
		}
	}
	for _, m := range policyResBindingCaptureRE.FindAllStringSubmatchIndex(body, -1) {
		binding := body[m[2]:m[3]]
		if lbl, ok := policyLabelForBinding(source, binding); ok {
			labels = append(labels, lbl)
		}
	}
	return labels
}

// resolvableLabelsInSource returns every standalone-policy label referenced
// anywhere in the file — direct `new formae.PolicyResolvable { label = "X" }`
// entries and `<binding>.res` references — regardless of which stack they sit
// in. Used to detect attachments that only exist in source (not yet applied),
// e.g. before deleting a policy whose references would otherwise dangle.
func resolvableLabelsInSource(source string) []string {
	var labels []string
	for _, m := range policyResolvableEntryRE.FindAllStringIndex(source, -1) {
		openIdx := strings.Index(source[m[0]:], "{") + m[0]
		closeIdx, ok := matchBrace(source, openIdx)
		if !ok {
			continue
		}
		if lm := stackLabelRE.FindStringSubmatch(source[openIdx+1 : closeIdx]); lm != nil {
			labels = append(labels, lm[1])
		}
	}
	for _, m := range policyResBindingCaptureRE.FindAllStringSubmatchIndex(source, -1) {
		binding := source[m[2]:m[3]]
		if lbl, ok := policyLabelForBinding(source, binding); ok {
			labels = append(labels, lbl)
		}
	}
	return labels
}
