package featuregate

import (
	"errors"
	"os"
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
	detectFn = func(string) (string, error) { return "0.86.0", nil }
	err := GuardFeature(FeatureProfile, "/usr/local/bin/formae")
	if err == nil || !strings.Contains(err.Error(), "requires formae >= 0.87.0") {
		t.Fatalf("expected version-floor error, got %v", err)
	}

	// new enough
	resetCacheForTest()
	detectFn = func(string) (string, error) { return "0.87.0", nil }
	if err := GuardFeature(FeatureProfile, "/usr/local/bin/formae"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// detection error
	resetCacheForTest()
	detectFn = func(string) (string, error) { return "", errors.New("boom") }
	if err := GuardFeature(FeatureProfile, "/usr/local/bin/formae"); err == nil {
		t.Fatal("expected error when detection fails")
	}
}

func TestDetectCaches(t *testing.T) {
	t.Cleanup(resetCacheForTest)
	resetCacheForTest()
	calls := 0
	detectFn = func(string) (string, error) { calls++; return "0.87.0", nil }
	_, _ = Detect("/a/formae")
	_, _ = Detect("/a/formae")
	if calls != 1 {
		t.Errorf("expected detectFn called once, got %d", calls)
	}
}

func TestDetectIsKeyedByBinary(t *testing.T) {
	resetCacheForTest()
	calls := map[string]int{}
	detectFn = func(bin string) (string, error) { calls[bin]++; return "0.90.0", nil }

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
	detectFn = func(bin string) (string, error) { calls++; return "0.90.0", nil }

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
