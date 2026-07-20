package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// minPolicySchemaVersion is the project-side twin of
// featuregate.FeatureStandalonePolicy (both 0.82.0): this gates the PklProject
// *schema* pin, featuregate gates the local formae *binary*. Keep them in sync.
//
// minPolicySchemaVersion is the oldest @formae PKL schema that defines
// TTLPolicy, AutoReconcilePolicy and PolicyResolvable. Confirmed against the
// formae repo: the policy schema commit (60599e79) first ships in tag 0.82.0.
// Projects pinning older fail with "Cannot find type `TTLPolicy` in module
// `Formae`." — an opaque PKL trace we translate into an actionable message.
const minPolicySchemaVersion = "0.82.0"

// policySchemaTooOldError reports a PklProject pinning a formae schema that
// predates policy support.
type policySchemaTooOldError struct {
	Found       string
	Minimum     string
	ProjectPath string
}

func (e *policySchemaTooOldError) Error() string {
	return fmt.Sprintf(
		"this project pins formae@%s, which predates policy support; policies require formae@%s or newer. "+
			"Bump the \"formae\" dependency in %s, then re-run `pkl project resolve` so PklProject.deps.json picks up the new schema.",
		e.Found, e.Minimum, e.ProjectPath,
	)
}

// formaeDepVersionRE extracts the pinned version from a package URI such as
// package://hub.platform.engineering/plugins/pkl/schema/pkl/formae/formae@0.87.1
var formaeDepVersionRE = regexp.MustCompile(`formae@(\d+\.\d+\.\d+)`)

// parseFormaeSchemaVersion returns the formae schema version pinned in a
// PklProject source. Returns ok=false when the dependency is absent or is
// expressed as a local import(...) with no version — callers fail open.
func parseFormaeSchemaVersion(pklProjectSource string) (string, bool) {
	m := formaeDepVersionRE.FindStringSubmatch(pklProjectSource)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// compareSemver compares dotted numeric versions component-wise. Returns
// negative if a < b, zero if equal, positive if a > b. Non-numeric or
// short components are treated as 0.
func compareSemver(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		av, bv := 0, 0
		if i < len(aParts) {
			av, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bv, _ = strconv.Atoi(bParts[i])
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}

// findPklProject walks up from startDir looking for a PklProject file.
// Returns the file path and ok=true on the first hit.
func findPklProject(startDir string) (string, bool) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "PklProject")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// checkPolicySchemaSupport verifies that the PklProject governing formaFilePath
// pins a formae schema new enough to define the policy types. It fails open:
// a missing PklProject, an unparseable dependency, or an unreadable file all
// return nil. Only a confidently-too-old pin produces an error.
func checkPolicySchemaSupport(formaFilePath string) error {
	projectPath, ok := findPklProject(filepath.Dir(formaFilePath))
	if !ok {
		return nil
	}
	source, err := os.ReadFile(projectPath)
	if err != nil {
		return nil
	}
	version, ok := parseFormaeSchemaVersion(string(source))
	if !ok {
		return nil
	}
	if compareSemver(version, minPolicySchemaVersion) < 0 {
		return &policySchemaTooOldError{
			Found:       version,
			Minimum:     minPolicySchemaVersion,
			ProjectPath: projectPath,
		}
	}
	return nil
}
