package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestWalkPKLFiles(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	files, err := walkPKLFiles(root)
	if err != nil {
		t.Fatalf("walkPKLFiles failed: %v", err)
	}
	sort.Strings(files)

	wantSuffixes := []string{
		filepath.Join("walker_fixture", "main.pkl"),
		filepath.Join("walker_fixture", "nested", "sub.pkl"),
	}
	if len(files) != len(wantSuffixes) {
		t.Fatalf("expected %d files, got %d: %v", len(wantSuffixes), len(files), files)
	}
	for i, want := range wantSuffixes {
		if !strings.HasSuffix(files[i], want) {
			t.Errorf("file %d: %q does not end with %q", i, files[i], want)
		}
	}
}

func TestResolveStackFileSingleMatch(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		if strings.HasSuffix(path, filepath.Join("nested", "sub.pkl")) {
			return []byte(`{"Stacks":[{"Label":"lifeline"}]}`), nil
		}
		return []byte(`{"Stacks":[{"Label":"production"}]}`), nil
	}
	got, err := resolveStackFile(root, "lifeline", eval)
	if err != nil {
		t.Fatalf("resolveStackFile failed: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("nested", "sub.pkl")) {
		t.Errorf("expected nested/sub.pkl, got %s", got)
	}
}

func TestResolveStackFileNotFound(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"other"}]}`), nil
	}
	_, err := resolveStackFile(root, "lifeline", eval)
	var nfErr *stackNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected stackNotFoundError, got %T: %v", err, err)
	}
}

func TestResolveStackFileMultipleMatches(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"lifeline"}]}`), nil
	}
	_, err := resolveStackFile(root, "lifeline", eval)
	var ambErr *stackAmbiguousError
	if !errors.As(err, &ambErr) {
		t.Fatalf("expected stackAmbiguousError, got %T: %v", err, err)
	}
	if len(ambErr.Candidates) < 2 {
		t.Errorf("expected at least 2 candidates, got %v", ambErr.Candidates)
	}
}

func TestResolveStackFileEvalErrorsSkipped(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		if strings.HasSuffix(path, filepath.Join("nested", "sub.pkl")) {
			return []byte(`{"Stacks":[{"Label":"lifeline"}]}`), nil
		}
		return nil, fmt.Errorf("malformed PKL")
	}
	got, err := resolveStackFile(root, "lifeline", eval)
	if err != nil {
		t.Fatalf("resolveStackFile failed: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("nested", "sub.pkl")) {
		t.Errorf("expected nested/sub.pkl, got %s", got)
	}
}

func TestResolveStandalonePolicyFileSingleMatch(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		if strings.HasSuffix(path, filepath.Join("nested", "sub.pkl")) {
			return []byte(`{"Stacks":[],"Policies":[{"Label":"ephemeral-1h","Type":"ttl"}]}`), nil
		}
		return []byte(`{"Stacks":[{"Label":"production"}],"Policies":[]}`), nil
	}
	got, err := resolveStandalonePolicyFile(root, "ephemeral-1h", eval)
	if err != nil {
		t.Fatalf("resolveStandalonePolicyFile failed: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("nested", "sub.pkl")) {
		t.Errorf("got:\n%s\nwant:\na path ending in nested/sub.pkl", got)
	}
}

func TestResolveStandalonePolicyFileNotFound(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[],"Policies":[{"Label":"other","Type":"ttl"}]}`), nil
	}
	_, err := resolveStandalonePolicyFile(root, "ephemeral-1h", eval)
	var nfErr *policySourceNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("got:\n%T (%v)\nwant:\n*policySourceNotFoundError", err, err)
	}
}

func TestResolveStandalonePolicyFileAmbiguous(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[],"Policies":[{"Label":"ephemeral-1h","Type":"ttl"}]}`), nil
	}
	_, err := resolveStandalonePolicyFile(root, "ephemeral-1h", eval)
	var ambErr *policySourceAmbiguousError
	if !errors.As(err, &ambErr) {
		t.Fatalf("got:\n%T (%v)\nwant:\n*policySourceAmbiguousError", err, err)
	}
	if len(ambErr.Candidates) < 2 {
		t.Errorf("got:\n%v\nwant:\nat least 2 candidates", ambErr.Candidates)
	}
}

func TestFormaJSONHasPolicyIgnoresStackLabels(t *testing.T) {
	// A stack named "ephemeral-1h" must not be mistaken for a policy of that name.
	formaJSON := []byte(`{"Stacks":[{"Label":"ephemeral-1h"}],"Policies":[]}`)
	if formaJSONHasPolicy(formaJSON, "ephemeral-1h") {
		t.Error("got:\ntrue\nwant:\nfalse (the label belongs to a stack, not a policy)")
	}
}

func TestResolveMainFormaFilePicksMostStacks(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "main.pkl") {
			return []byte(`{"Stacks":[{"Label":"a"},{"Label":"b"},{"Label":"c"}]}`), nil
		}
		return []byte(`{"Stacks":[{"Label":"d"}]}`), nil
	}
	got, err := resolveMainFormaFile(root, eval)
	if err != nil {
		t.Fatalf("resolveMainFormaFile failed: %v", err)
	}
	if !strings.HasSuffix(got, "main.pkl") {
		t.Errorf("got:\n%s\nwant:\na path ending in main.pkl", got)
	}
}

func TestResolveMainFormaFileTieIsAmbiguous(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"a"},{"Label":"b"}]}`), nil
	}
	_, err := resolveMainFormaFile(root, eval)
	var ambErr *mainFormaFileAmbiguousError
	if !errors.As(err, &ambErr) {
		t.Fatalf("got:\n%T (%v)\nwant:\n*mainFormaFileAmbiguousError", err, err)
	}
	if len(ambErr.Candidates) < 2 {
		t.Errorf("got:\n%v\nwant:\nat least 2 tied candidates", ambErr.Candidates)
	}
	if ambErr.StackCount != 2 {
		t.Errorf("got:\n%d\nwant:\n%d", ambErr.StackCount, 2)
	}
}

func TestResolveMainFormaFileNoStacksAnywhere(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[]}`), nil
	}
	_, err := resolveMainFormaFile(root, eval)
	var nfErr *mainFormaFileNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("got:\n%T (%v)\nwant:\n*mainFormaFileNotFoundError", err, err)
	}
}

func TestResolveMainFormaFileSkipsUnevaluableFiles(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "policy", "walker_fixture")
	eval := func(path string) ([]byte, error) {
		if strings.HasSuffix(path, filepath.Join("nested", "sub.pkl")) {
			return []byte(`{"Stacks":[{"Label":"a"}]}`), nil
		}
		return nil, fmt.Errorf("malformed PKL")
	}
	got, err := resolveMainFormaFile(root, eval)
	if err != nil {
		t.Fatalf("resolveMainFormaFile failed: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("nested", "sub.pkl")) {
		t.Errorf("got:\n%s\nwant:\na path ending in nested/sub.pkl", got)
	}
}
