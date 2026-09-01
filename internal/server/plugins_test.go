package server

import "testing"

// A -dev build publishes its schema package over the canonical X.Y.Z
// coordinate, so that is the version a project pins. No other prerelease
// carries that convention, and naming one would point at a different
// published version.
func TestCanonicalSchemaVersion(t *testing.T) {
	cases := []struct {
		installed string
		want      string
		wantOK    bool
	}{
		{"0.1.10", "0.1.10", true},
		{"0.1.17-dev.8", "0.1.17", true},
		{"0.1.17-dev", "0.1.17", true},
		{"1.2.3-rc.1", "", false},
		{"1.2.3-beta.2", "", false},
		{"1.2.3-dev.4.5", "", false},
		{"0.1", "", false},
		{"latest", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := canonicalSchemaVersion(tc.installed)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("canonicalSchemaVersion(%q) = (%q, %v), want (%q, %v)",
				tc.installed, got, ok, tc.want, tc.wantOK)
		}
	}
}
