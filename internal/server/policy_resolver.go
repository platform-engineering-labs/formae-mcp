package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
)

// EvalFunc evaluates a single PKL file and returns its JSON output.
type EvalFunc func(path string) ([]byte, error)

// stackNotFoundError indicates no PKL file declared the requested stack.
type stackNotFoundError struct {
	Stack string
}

func (e *stackNotFoundError) Error() string {
	return fmt.Sprintf("no PKL file in workspace declares stack %q", e.Stack)
}

// stackAmbiguousError indicates multiple PKL files declared the same stack label.
type stackAmbiguousError struct {
	Stack      string
	Candidates []string
}

func (e *stackAmbiguousError) Error() string {
	return fmt.Sprintf("multiple PKL files declare stack %q: %v", e.Stack, e.Candidates)
}

// skippedDirs are directories that walkPKLFiles never recurses into.
var skippedDirs = map[string]bool{
	"node_modules": true,
	".formae":      true,
	".git":         true,
}

// walkPKLFiles returns absolute paths of all .pkl files under root, skipping
// vendored, formae-managed, hidden, and version-control directories.
func walkPKLFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (skippedDirs[name] || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".pkl" {
			return nil
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return fmt.Errorf("abs path: %w", absErr)
		}
		files = append(files, abs)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// formaPredicate answers a yes/no question about an evaluated forma document.
type formaPredicate func(formaJSON []byte) bool

// resolveFormaFileBy walks the workspace, evaluates each PKL file, and returns
// every file whose forma satisfies pred, in walk order. Files that fail to
// evaluate are skipped silently — a workspace routinely contains PKL modules
// that are not standalone formae (vars, templates, partial imports).
func resolveFormaFileBy(root string, eval EvalFunc, pred formaPredicate) ([]string, error) {
	files, err := walkPKLFiles(root)
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}
	var matches []string
	for _, file := range files {
		out, evalErr := eval(file)
		if evalErr != nil {
			continue
		}
		if pred(out) {
			matches = append(matches, file)
		}
	}
	return matches, nil
}

// resolveStackFile returns the single PKL file declaring the named stack.
// Returns *stackNotFoundError if no file declares it, *stackAmbiguousError if
// more than one does.
func resolveStackFile(root, stackLabel string, eval EvalFunc) (string, error) {
	matches, err := resolveFormaFileBy(root, eval, func(formaJSON []byte) bool {
		return formaJSONHasStack(formaJSON, stackLabel)
	})
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", &stackNotFoundError{Stack: stackLabel}
	case 1:
		return matches[0], nil
	default:
		return "", &stackAmbiguousError{Stack: stackLabel, Candidates: matches}
	}
}

// formaJSONHasStack returns true if the given forma JSON declares a stack with
// the given label.
func formaJSONHasStack(formaJSON []byte, label string) bool {
	var f struct {
		Stacks []struct {
			Label string `json:"Label"`
		} `json:"Stacks"`
	}
	if err := json.Unmarshal(formaJSON, &f); err != nil {
		return false
	}
	for _, s := range f.Stacks {
		if s.Label == label {
			return true
		}
	}
	return false
}

// formaeEval is the production EvalFunc — invokes `formae eval` on the file.
func formaeEval(path string) ([]byte, error) {
	cmd := exec.Command("formae", "eval", path, "--output-schema", "json", "--output-consumer", "machine")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("formae eval failed for %s: %s", path, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("formae eval failed for %s: %w", path, err)
	}
	return out, nil
}

// policySourceNotFoundError indicates no PKL file in the workspace declares the
// named standalone policy. The agent may still know about it — the source is
// what is missing.
type policySourceNotFoundError struct {
	Policy string
}

func (e *policySourceNotFoundError) Error() string {
	return fmt.Sprintf("no PKL file in the workspace declares standalone policy %q "+
		"(the agent knows the policy, but its source declaration could not be located)", e.Policy)
}

// policySourceAmbiguousError indicates multiple PKL files declare the same
// standalone policy label.
type policySourceAmbiguousError struct {
	Policy     string
	Candidates []string
}

func (e *policySourceAmbiguousError) Error() string {
	return fmt.Sprintf("multiple PKL files declare standalone policy %q: %v", e.Policy, e.Candidates)
}

// formaJSONHasPolicy reports whether the evaluated forma declares a standalone
// policy with the given label. Only the top-level Policies array is consulted —
// inline policies live under Stacks[].Policies and must not match.
func formaJSONHasPolicy(formaJSON []byte, label string) bool {
	var f struct {
		Policies []struct {
			Label string `json:"Label"`
		} `json:"Policies"`
	}
	if err := json.Unmarshal(formaJSON, &f); err != nil {
		return false
	}
	for _, p := range f.Policies {
		if p.Label == label {
			return true
		}
	}
	return false
}

// resolveStandalonePolicyFile returns the single PKL file declaring the named
// standalone policy. Mirrors resolveStackFile over the Policies array.
func resolveStandalonePolicyFile(root, policyLabel string, eval EvalFunc) (string, error) {
	matches, err := resolveFormaFileBy(root, eval, func(formaJSON []byte) bool {
		return formaJSONHasPolicy(formaJSON, policyLabel)
	})
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", &policySourceNotFoundError{Policy: policyLabel}
	case 1:
		return matches[0], nil
	default:
		return "", &policySourceAmbiguousError{Policy: policyLabel, Candidates: matches}
	}
}

// mainFormaFileNotFoundError indicates no evaluable PKL file in the workspace
// declares any stack, so there is nowhere sensible to put a standalone policy.
type mainFormaFileNotFoundError struct{}

func (e *mainFormaFileNotFoundError) Error() string {
	return "no forma file with stacks found in the workspace; " +
		"pass forma_file explicitly to say where the standalone policy should be declared"
}

// mainFormaFileAmbiguousError indicates several files tie for "most stacks".
// The skill is expected to present the candidates and ask the user once.
type mainFormaFileAmbiguousError struct {
	Candidates []string
	StackCount int
}

func (e *mainFormaFileAmbiguousError) Error() string {
	return fmt.Sprintf("cannot identify a single main forma file: %d files each declare %d stacks: %v. "+
		"Pass forma_file explicitly to choose one", len(e.Candidates), e.StackCount, e.Candidates)
}

// countStacksInFormaJSON returns the number of stacks an evaluated forma
// declares. Returns 0 on parse failure.
func countStacksInFormaJSON(formaJSON []byte) int {
	var f struct {
		Stacks []struct {
			Label string `json:"Label"`
		} `json:"Stacks"`
	}
	if err := json.Unmarshal(formaJSON, &f); err != nil {
		return 0
	}
	return len(f.Stacks)
}

// resolveMainFormaFile picks the workspace's main forma file — the one
// declaring the most stacks. Files that fail to evaluate or declare no stacks
// are ignored. A tie for the top count is an ambiguity error, never a guess.
func resolveMainFormaFile(root string, eval EvalFunc) (string, error) {
	files, err := walkPKLFiles(root)
	if err != nil {
		return "", fmt.Errorf("walk workspace: %w", err)
	}

	best := 0
	var winners []string
	for _, file := range files {
		out, evalErr := eval(file)
		if evalErr != nil {
			continue
		}
		count := countStacksInFormaJSON(out)
		if count == 0 {
			continue
		}
		switch {
		case count > best:
			best = count
			winners = []string{file}
		case count == best:
			winners = append(winners, file)
		}
	}

	switch {
	case len(winners) == 0:
		return "", &mainFormaFileNotFoundError{}
	case len(winners) == 1:
		return winners[0], nil
	default:
		return "", &mainFormaFileAmbiguousError{Candidates: winners, StackCount: best}
	}
}
