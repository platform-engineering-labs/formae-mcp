package featuregate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrTooOld marks a gate refusing an install below its floor. Callers match on
// it to tell "your formae is too old" apart from every other resolution
// failure, which is the difference between an actionable upgrade prompt and an
// opaque error.
var ErrTooOld = errors.New("formae is too old")

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

// FeatureConnectionOracle is reading a profile's resolved configuration from
// the CLI rather than parsing it here. There is one formae per machine and no
// second binary to fall back to, so an older install is a clear upgrade prompt
// rather than a silent downgrade.
const FeatureConnectionOracle Feature = "connection-oracle"

// detectTimeout applies when the caller supplies no deadline of its own, so
// version detection is bounded even on the context-free path.
const detectTimeout = 10 * time.Second

// registry maps each feature to its minimum required formae version.
var registry = map[Feature]string{
	FeatureConnectionOracle:    "0.89.0",
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
	detectFn = detectFromCLI // func(ctx context.Context, bin string) (string, error)
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
	detectFn = func(context.Context, string) (string, error) { return v, nil }
	cache = map[string]result{}
}

// fileIdentityKey returns a cache key that includes the file's mtime and size
// so an in-place binary upgrade (same path, new build) invalidates the entry.
// If stat fails (path does not exist, permissions, etc.) it falls back to the
// plain path so callers with synthetic/stub paths still work.
func fileIdentityKey(bin string) string {
	fi, err := os.Stat(bin)
	if err != nil {
		return bin
	}
	return fmt.Sprintf("%s\x00%d\x00%d", bin, fi.ModTime().UnixNano(), fi.Size())
}

// Detect returns the version of the formae binary at bin. It is DetectContext
// with no deadline of the caller's own.
func Detect(bin string) (string, error) {
	return DetectContext(context.Background(), bin)
}

// DetectContext returns the version of the formae binary at bin, memoized by
// file identity (path + mtime + size). An in-place binary upgrade (same path,
// new build) causes re-detection on the next call. When stat is unavailable the
// key falls back to the plain path.
//
// The subprocess runs outside the package mutex: holding it would let one slow
// or hung formae block every other gated call.
func DetectContext(ctx context.Context, bin string) (string, error) {
	key := fileIdentityKey(bin)

	cacheMu.Lock()
	r, cached := cache[key]
	detect := detectFn
	cacheMu.Unlock()
	if cached {
		return r.ver, r.err
	}

	ver, err := detect(ctx, bin)

	// A cancelled or timed-out detection says nothing about the binary. Caching
	// it would make one abandoned tool call fail every gated call afterwards.
	if ctx.Err() == nil {
		cacheMu.Lock()
		cache[key] = result{ver, err}
		cacheMu.Unlock()
	}
	return ver, err
}

// GuardFeature returns nil if the formae binary at bin satisfies the feature's
// minimum version, else a "requires formae >= X.Y.Z (connected: A.B.C)" error.
func GuardFeature(f Feature, bin string) error {
	return GuardFeatureContext(context.Background(), f, bin)
}

// GuardFeatureContext is GuardFeature bounded by ctx, so a cancelled tool call
// stops the version probe instead of leaving it running.
func GuardFeatureContext(ctx context.Context, f Feature, bin string) error {
	min, ok := registry[f]
	if !ok {
		return fmt.Errorf("unknown feature %q", f)
	}
	got, err := DetectContext(ctx, bin)
	if err != nil {
		return fmt.Errorf("could not determine formae version: %w", err)
	}
	if CompareVersions(got, min) < 0 {
		return fmt.Errorf("%w: requires formae >= %s (connected: %s)", ErrTooOld, min, got)
	}
	return nil
}

func detectFromCLI(ctx context.Context, bin string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, detectTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, bin, "--version")
	// Own process group. Cancelling exec.CommandContext kills only the immediate
	// child, and CombinedOutput waits for EOF on descriptors a grandchild can
	// still hold, so the call would hang anyway.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	out, err := cmd.CombinedOutput()
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

// CompareVersions returns -1, 0, or 1 comparing two X.Y.Z strings numerically.
func CompareVersions(a, b string) int {
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
