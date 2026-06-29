package featuregate

import (
	"errors"
	"strings"
	"testing"
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
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGuardFeature(t *testing.T) {
	t.Cleanup(resetCacheForTest)

	// too old
	resetCacheForTest()
	detectFn = func() (string, error) { return "0.86.0", nil }
	err := GuardFeature(FeatureProfile)
	if err == nil || !strings.Contains(err.Error(), "requires formae >= 0.87.0") {
		t.Fatalf("expected version-floor error, got %v", err)
	}

	// new enough
	resetCacheForTest()
	detectFn = func() (string, error) { return "0.87.0", nil }
	if err := GuardFeature(FeatureProfile); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// detection error
	resetCacheForTest()
	detectFn = func() (string, error) { return "", errors.New("boom") }
	if err := GuardFeature(FeatureProfile); err == nil {
		t.Fatal("expected error when detection fails")
	}
}

func TestDetectCaches(t *testing.T) {
	t.Cleanup(resetCacheForTest)
	resetCacheForTest()
	calls := 0
	detectFn = func() (string, error) { calls++; return "0.87.0", nil }
	_, _ = Detect()
	_, _ = Detect()
	if calls != 1 {
		t.Errorf("expected detectFn called once, got %d", calls)
	}
}
