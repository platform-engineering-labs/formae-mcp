package main

import (
	"bytes"
	"testing"
)

func TestTryVersion_PrintsVersionAndReports(t *testing.T) {
	for _, arg := range []string{"--version", "-version"} {
		var buf bytes.Buffer
		if !tryVersion([]string{arg}, &buf) {
			t.Errorf("tryVersion([%q]) = false, want true", arg)
		}
		if got := buf.String(); got != version+"\n" {
			t.Errorf("tryVersion([%q]) wrote %q, want %q", arg, got, version+"\n")
		}
	}
}

func TestTryVersion_NoFlag(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"something"}} {
		var buf bytes.Buffer
		if tryVersion(args, &buf) {
			t.Errorf("tryVersion(%q) = true, want false", args)
		}
		if got := buf.String(); got != "" {
			t.Errorf("tryVersion(%q) wrote %q, want empty", args, got)
		}
	}
}

func TestVersion_DefaultsToDev(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want %q (default when not injected)", version, "dev")
	}
}
