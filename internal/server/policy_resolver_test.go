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
