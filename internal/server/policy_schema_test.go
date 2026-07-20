package server

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFormaeSchemaVersion(t *testing.T) {
	source := `amends "pkl:Project"

dependencies {
  ["formae"] {
    uri = "package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.80.1"
  }
  ["aws"] {
    uri = "package://hub.platform.engineering/plugins/aws/schema/pkl/aws/aws@0.1.0"
  }
}
`
	got, ok := parseFormaeSchemaVersion(source)
	if !ok {
		t.Fatal("expected to parse a formae schema version")
	}
	if got != "0.80.1" {
		t.Errorf("got:\n%s\nwant:\n%s", got, "0.80.1")
	}
}

func TestParseFormaeSchemaVersionLocalImportIsUnparseable(t *testing.T) {
	// tiwa-demo style: the formae dep resolved from a local path has no @version.
	source := `amends "pkl:Project"

dependencies {
  ["formae"] = import("/opt/pel/formae/schema/pkl/PklProject")
}
`
	if _, ok := parseFormaeSchemaVersion(source); ok {
		t.Error("got:\nparsed a version\nwant:\nno version (fail open)")
	}
}

func TestCompareSemver(t *testing.T) {
	if compareSemver("0.80.1", "0.82.0") >= 0 {
		t.Error("got:\n0.80.1 >= 0.82.0\nwant:\n0.80.1 < 0.82.0")
	}
	if compareSemver("0.82.0", "0.82.0") != 0 {
		t.Error("got:\n0.82.0 != 0.82.0\nwant:\nequal")
	}
	if compareSemver("0.87.1", "0.82.0") <= 0 {
		t.Error("got:\n0.87.1 <= 0.82.0\nwant:\n0.87.1 > 0.82.0")
	}
	// Numeric, not lexical: 0.9.0 < 0.82.0 lexically, but 0.9.0 > 0.82.0 is false numerically.
	if compareSemver("0.9.0", "0.82.0") >= 0 {
		t.Error("got:\n0.9.0 >= 0.82.0\nwant:\n0.9.0 < 0.82.0 (minor compared numerically)")
	}
}

func TestCheckPolicySchemaSupportTooOld(t *testing.T) {
	dir := t.TempDir()
	pklProject := `amends "pkl:Project"

dependencies {
  ["formae"] {
    uri = "package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.80.1"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "PklProject"), []byte(pklProject), 0o644); err != nil {
		t.Fatalf("write PklProject: %v", err)
	}
	formaFile := filepath.Join(dir, "main.pkl")
	if err := os.WriteFile(formaFile, []byte("// forma\n"), 0o644); err != nil {
		t.Fatalf("write main.pkl: %v", err)
	}

	err := checkPolicySchemaSupport(formaFile)
	var tooOld *policySchemaTooOldError
	if !errors.As(err, &tooOld) {
		t.Fatalf("got:\n%T (%v)\nwant:\n*policySchemaTooOldError", err, err)
	}
	if tooOld.Found != "0.80.1" {
		t.Errorf("got:\n%s\nwant:\n%s", tooOld.Found, "0.80.1")
	}
	if tooOld.Minimum != minPolicySchemaVersion {
		t.Errorf("got:\n%s\nwant:\n%s", tooOld.Minimum, minPolicySchemaVersion)
	}
}

func TestCheckPolicySchemaSupportNewEnough(t *testing.T) {
	dir := t.TempDir()
	pklProject := `amends "pkl:Project"

dependencies {
  ["formae"] {
    uri = "package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.87.1"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "PklProject"), []byte(pklProject), 0o644); err != nil {
		t.Fatalf("write PklProject: %v", err)
	}
	formaFile := filepath.Join(dir, "main.pkl")
	if err := os.WriteFile(formaFile, []byte("// forma\n"), 0o644); err != nil {
		t.Fatalf("write main.pkl: %v", err)
	}

	if err := checkPolicySchemaSupport(formaFile); err != nil {
		t.Errorf("got:\n%v\nwant:\nnil", err)
	}
}

func TestCheckPolicySchemaSupportFindsProjectInParentDir(t *testing.T) {
	dir := t.TempDir()
	pklProject := `amends "pkl:Project"

dependencies {
  ["formae"] {
    uri = "package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.80.1"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "PklProject"), []byte(pklProject), 0o644); err != nil {
		t.Fatalf("write PklProject: %v", err)
	}
	nested := filepath.Join(dir, "stacks", "prod")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	formaFile := filepath.Join(nested, "main.pkl")
	if err := os.WriteFile(formaFile, []byte("// forma\n"), 0o644); err != nil {
		t.Fatalf("write main.pkl: %v", err)
	}

	var tooOld *policySchemaTooOldError
	if err := checkPolicySchemaSupport(formaFile); !errors.As(err, &tooOld) {
		t.Fatalf("got:\n%T (%v)\nwant:\n*policySchemaTooOldError from the parent PklProject", err, err)
	}
}

func TestCheckPolicySchemaSupportNoProjectFailsOpen(t *testing.T) {
	dir := t.TempDir()
	formaFile := filepath.Join(dir, "main.pkl")
	if err := os.WriteFile(formaFile, []byte("// forma\n"), 0o644); err != nil {
		t.Fatalf("write main.pkl: %v", err)
	}

	if err := checkPolicySchemaSupport(formaFile); err != nil {
		t.Errorf("got:\n%v\nwant:\nnil (no PklProject means fail open)", err)
	}
}
