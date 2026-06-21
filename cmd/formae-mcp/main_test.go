package main

import (
	"bytes"
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if version != "dev" {
		t.Errorf("expected default version 'dev', got '%s'", version)
	}
}

func TestHandleVersion_LongFlag(t *testing.T) {
	defer func(orig string) { version = orig }(version)
	version = "1.2.3"

	var buf bytes.Buffer
	if handled := handleVersion([]string{"--version"}, &buf); !handled {
		t.Error("expected handleVersion to return true for '--version'")
	}
	if got := buf.String(); got != "1.2.3\n" {
		t.Errorf("expected output '1.2.3\\n', got '%s'", got)
	}
}

func TestHandleVersion_SingleDashFlag(t *testing.T) {
	defer func(orig string) { version = orig }(version)
	version = "1.2.3"

	var buf bytes.Buffer
	if handled := handleVersion([]string{"-version"}, &buf); !handled {
		t.Error("expected handleVersion to return true for '-version'")
	}
	if got := buf.String(); got != "1.2.3\n" {
		t.Errorf("expected output '1.2.3\\n', got '%s'", got)
	}
}

func TestHandleVersion_AmongOtherArgs(t *testing.T) {
	var buf bytes.Buffer
	if handled := handleVersion([]string{"--foo", "--version", "bar"}, &buf); !handled {
		t.Error("expected handleVersion to return true when '--version' is present among other args")
	}
}

func TestHandleVersion_Absent(t *testing.T) {
	var buf bytes.Buffer
	if handled := handleVersion([]string{}, &buf); handled {
		t.Error("expected handleVersion to return false for empty args")
	}
	if got := buf.String(); got != "" {
		t.Errorf("expected no output, got '%s'", got)
	}
}
