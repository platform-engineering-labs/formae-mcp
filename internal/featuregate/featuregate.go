package featuregate

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Feature names a version-gated MCP capability.
type Feature string

// FeatureProfile gates all profile management and the per-invocation --profile.
const FeatureProfile Feature = "profile"

// FeatureStandalonePolicy gates the standalone (reusable) policy tools:
// create/attach/detach/delete_standalone_policy. The policy system (schema +
// agent attachment support) first shipped in formae 0.82.0. This gates the
// local formae *binary*; its project-side twin is server.minPolicySchemaVersion
// (the PklProject schema pin), also 0.82.0 — keep them in sync.
const FeatureStandalonePolicy Feature = "standalone-policy"

// FeatureAutoReconcilePolicy gates the auto-reconcile policy *type* in both the
// inline and standalone policy tools. Before formae 0.88.0 the agent's policy
// update generator mishandled auto-reconcile policies (a phantom update on
// every apply, and an empty persisted label for inline ones); 0.88.0 fixes
// both. TTL policies are unaffected and are not gated by this.
const FeatureAutoReconcilePolicy Feature = "auto-reconcile-policy"

// registry maps each feature to its minimum required formae version.
var registry = map[Feature]string{
	FeatureProfile:             "0.87.0",
	FeatureStandalonePolicy:    "0.82.0",
	FeatureAutoReconcilePolicy: "0.88.0",
}

type result struct {
	ver string
	err error
}

var (
	cacheMu  sync.Mutex
	cache    = map[string]result{}
	detectFn = detectFromCLI // func(bin string) (string, error)
)

// resetCacheForTest clears the per-binary version cache (tests only).
func resetCacheForTest() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = map[string]result{}
	detectFn = detectFromCLI
}

// SetDetectForTest forces a version for any binary and clears the cache.
func SetDetectForTest(v string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	detectFn = func(string) (string, error) { return v, nil }
	cache = map[string]result{}
}

// Detect returns the version of the formae binary at bin, memoized per binary
// path so a mid-process binary swap is not masked by a stale cache entry.
func Detect(bin string) (string, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if r, ok := cache[bin]; ok {
		return r.ver, r.err
	}
	ver, err := detectFn(bin)
	cache[bin] = result{ver, err}
	return ver, err
}

// GuardFeature returns nil if the formae binary at bin satisfies the feature's
// minimum version, else a "requires formae >= X.Y.Z (connected: A.B.C)" error.
func GuardFeature(f Feature, bin string) error {
	min, ok := registry[f]
	if !ok {
		return fmt.Errorf("unknown feature %q", f)
	}
	got, err := Detect(bin)
	if err != nil {
		return fmt.Errorf("could not determine formae version: %w", err)
	}
	if compareVersions(got, min) < 0 {
		return fmt.Errorf("requires formae >= %s (connected: %s)", min, got)
	}
	return nil
}

func detectFromCLI(bin string) (string, error) {
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("formae --version failed: %w (output: %s)", err, string(out))
	}
	return parseFormaeVersion(string(out))
}

var versionLineRe = regexp.MustCompile(`formae version:\s*([0-9]+\.[0-9]+\.[0-9]+)`)

func parseFormaeVersion(out string) (string, error) {
	m := versionLineRe.FindStringSubmatch(out)
	if len(m) < 2 {
		return "", fmt.Errorf("could not parse formae version from %q", strings.TrimSpace(out))
	}
	return m[1], nil
}

// compareVersions returns -1, 0, or 1 comparing two X.Y.Z strings numerically.
func compareVersions(a, b string) int {
	pa, pb := parseParts(a), parseParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseParts(v string) [3]int {
	var out [3]int
	for i, s := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		out[i] = n
	}
	return out
}
