package main

import (
	"bytes"
	"testing"
)

func TestTryVersion_PrintsVersionAndReports(t *testing.T) {
	const v = "1.2.3"
	for _, arg := range []string{"--version", "-version"} {
		var buf bytes.Buffer
		if !tryVersion([]string{arg}, v, &buf) {
			t.Errorf("tryVersion([%q]) = false, want true", arg)
		}
		if got := buf.String(); got != v+"\n" {
			t.Errorf("tryVersion([%q]) wrote %q, want %q", arg, got, v+"\n")
		}
	}
}

func TestTryVersion_NoFlag(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"something"}} {
		var buf bytes.Buffer
		if tryVersion(args, "1.2.3", &buf) {
			t.Errorf("tryVersion(%q) = true, want false", args)
		}
		if got := buf.String(); got != "" {
			t.Errorf("tryVersion(%q) wrote %q, want empty", args, got)
		}
	}
}

func TestTryHelp_PrintsUsageAndReports(t *testing.T) {
	for _, arg := range []string{"--help", "-help", "-h"} {
		var buf bytes.Buffer
		if !tryHelp([]string{arg}, &buf) {
			t.Errorf("tryHelp([%q]) = false, want true", arg)
		}
		if got := buf.String(); got != usage {
			t.Errorf("tryHelp([%q]) wrote %q, want %q", arg, got, usage)
		}
	}
}

func TestTryHelp_NoFlag(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"something"}, {"--version"}} {
		var buf bytes.Buffer
		if tryHelp(args, &buf) {
			t.Errorf("tryHelp(%q) = true, want false", args)
		}
		if got := buf.String(); got != "" {
			t.Errorf("tryHelp(%q) wrote %q, want empty", args, got)
		}
	}
}
