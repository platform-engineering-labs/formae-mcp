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

// resolveStackFile walks the workspace, evaluates each PKL file, and returns
// the file path declaring the named stack. Files that fail to evaluate are
// skipped silently. Returns *stackNotFoundError if no file declares the stack,
// or *stackAmbiguousError if multiple files do.
func resolveStackFile(root, stackLabel string, eval EvalFunc) (string, error) {
	files, err := walkPKLFiles(root)
	if err != nil {
		return "", fmt.Errorf("walk workspace: %w", err)
	}
	var matches []string
	for _, file := range files {
		out, err := eval(file)
		if err != nil {
			continue
		}
		if formaJSONHasStack(out, stackLabel) {
			matches = append(matches, file)
		}
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
