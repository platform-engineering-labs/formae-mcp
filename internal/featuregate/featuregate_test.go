package featuregate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseFormaeVersion(t *testing.T) {
	out := "formae version: 0.87.0\ngo version: go1.26.2\n"
	got, err := parseFormaeVersion(out)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.87.0" {
		t.Errorf("expected 0.87.0, got %q", got)
	}
	if _, err := parseFormaeVersion("nonsense"); err == nil {
		t.Error("expected error for unparseable output")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.87.0", "0.87.0", 0},
		{"0.86.9", "0.87.0", -1},
		{"0.87.1", "0.87.0", 1},
		{"1.0.0", "0.87.0", 1},
		{"0.87.0", "0.87.1", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGuardFeature(t *testing.T) {
	t.Cleanup(resetCacheForTest)

	// too old
	resetCacheForTest()
	detectFn = func(context.Context, string) (string, error) { return "0.86.0", nil }
	err := GuardFeature(FeatureProfile, "/usr/local/bin/formae")
	if err == nil || !strings.Contains(err.Error(), "requires formae >= 0.87.0") {
		t.Fatalf("expected version-floor error, got %v", err)
	}

	// new enough
	resetCacheForTest()
	detectFn = func(context.Context, string) (string, error) { return "0.87.0", nil }
	if err := GuardFeature(FeatureProfile, "/usr/local/bin/formae"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// detection error
	resetCacheForTest()
	detectFn = func(context.Context, string) (string, error) { return "", errors.New("boom") }
	if err := GuardFeature(FeatureProfile, "/usr/local/bin/formae"); err == nil {
		t.Fatal("expected error when detection fails")
	}
}

func TestDetectCaches(t *testing.T) {
	t.Cleanup(resetCacheForTest)
	resetCacheForTest()
	calls := 0
	detectFn = func(context.Context, string) (string, error) { calls++; return "0.87.0", nil }
	_, _ = Detect("/a/formae")
	_, _ = Detect("/a/formae")
	if calls != 1 {
		t.Errorf("expected detectFn called once, got %d", calls)
	}
}

func TestDetectIsKeyedByBinary(t *testing.T) {
	resetCacheForTest()
	calls := map[string]int{}
	detectFn = func(_ context.Context, bin string) (string, error) { calls[bin]++; return "0.90.0", nil }

	_, _ = Detect("/a/formae")
	_, _ = Detect("/a/formae")
	_, _ = Detect("/b/formae")

	if calls["/a/formae"] != 1 || calls["/b/formae"] != 1 {
		t.Fatalf("expected per-binary memoization, got %v", calls)
	}
}

func TestDetectRerunsOnBinaryChange(t *testing.T) {
	t.Cleanup(resetCacheForTest)
	resetCacheForTest()

	// Write a temp file to stand in for the formae binary.
	f, err := os.CreateTemp("", "formae-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString("v1"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	calls := 0
	detectFn = func(_ context.Context, bin string) (string, error) { calls++; return "0.90.0", nil }

	_, _ = Detect(f.Name())
	if calls != 1 {
		t.Fatalf("expected 1 call after first detect, got %d", calls)
	}

	// Simulate an in-place upgrade: overwrite with different content and bump mtime.
	if err := os.WriteFile(f.Name(), []byte("v2-longer"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ensure mtime changes even on fast filesystems.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(f.Name(), future, future); err != nil {
		t.Fatal(err)
	}

	_, _ = Detect(f.Name())
	if calls != 2 {
		t.Fatalf("expected detectFn re-run after binary change, got %d calls", calls)
	}
}

func TestRegistry_ConnectionOracleFloor(t *testing.T) {
	if got := registry[FeatureConnectionOracle]; got != "0.89.0" {
		t.Fatalf("connection oracle floor: want %q, got %q", "0.89.0", got)
	}
}

// stubFormae writes an executable stand-in for the formae CLI running script.
func stubFormae(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "formae")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}
	return path
}

// TestGuardFeatureContext_HonoursCancellation pins that a hung formae cannot
// hold a tool call open. exec.CommandContext alone would not do it: detection
// reads until EOF, and a grandchild holding the pipe keeps it open.
func TestGuardFeatureContext_HonoursCancellation(t *testing.T) {
	t.Cleanup(resetCacheForTest)
	resetCacheForTest()

	bin := stubFormae(t, "sleep 30\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- GuardFeatureContext(ctx, FeatureConnectionOracle, bin) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("GuardFeatureContext with a cancelled context: expected an error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GuardFeatureContext did not return on a cancelled context")
	}
}

// TestDetectContext_DoesNotCacheACancelledDetection pins that a cancelled call
// says nothing about the binary. Caching its error would make one cancelled
// tool call fail every gated call for the rest of the process lifetime.
func TestDetectContext_DoesNotCacheACancelledDetection(t *testing.T) {
	t.Cleanup(resetCacheForTest)
	resetCacheForTest()

	bin := stubFormae(t, "echo 'formae version: 0.89.0'\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DetectContext(ctx, bin); err == nil {
		t.Fatal("DetectContext with a cancelled context: expected an error, got nil")
	}

	got, err := DetectContext(context.Background(), bin)
	if err != nil {
		t.Fatalf("DetectContext after a cancelled call: unexpected error: %v", err)
	}
	if got != "0.89.0" {
		t.Fatalf("version: want %q, got %q", "0.89.0", got)
	}
}
