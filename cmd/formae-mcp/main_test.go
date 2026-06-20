package main

import (
	"bytes"
	"testing"
)

func TestRun_LongVersionFlag(t *testing.T) {
	var buf bytes.Buffer
	handled := run([]string{"--version"}, &buf)
	if !handled {
		t.Fatal("expected run to handle --version, got handled=false")
	}
	if buf.String() != "dev\n" {
		t.Errorf("expected output 'dev\\n', got %q", buf.String())
	}
}

func TestRun_ShortVersionFlag(t *testing.T) {
	var buf bytes.Buffer
	handled := run([]string{"-version"}, &buf)
	if !handled {
		t.Fatal("expected run to handle -version, got handled=false")
	}
	if buf.String() != "dev\n" {
		t.Errorf("expected output 'dev\\n', got %q", buf.String())
	}
}

func TestRun_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	handled := run([]string{}, &buf)
	if handled {
		t.Error("expected run not to handle empty args, got handled=true")
	}
	if buf.String() != "" {
		t.Errorf("expected no output, got %q", buf.String())
	}
}

func TestRun_OtherArgsIgnored(t *testing.T) {
	var buf bytes.Buffer
	handled := run([]string{"--unknown", "foo"}, &buf)
	if handled {
		t.Error("expected run not to handle unrelated args, got handled=true")
	}
	if buf.String() != "" {
		t.Errorf("expected no output, got %q", buf.String())
	}
}
