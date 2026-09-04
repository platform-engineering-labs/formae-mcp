#!/usr/bin/env sh
# resolve-formae.sh — exercises resolve_formae from scripts/provision.sh.
#
# resolve_formae decides which formae binary the whole product shells out to,
# and whether an upgrade needs sudo, so it is checked here rather than only in
# the container install tests.

set -u

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. "$ROOT/scripts/provision.sh"

failures=0

fail() {
    echo "FAIL: $1" >&2
    failures=$((failures + 1))
}

expect() {
    _what="$1"
    _got="$2"
    _want="$3"
    [ "$_got" = "$_want" ] || fail "$_what: got '$_got', want '$_want'"
}

# stub_bin writes an executable no-op at the given path.
stub_bin() {
    mkdir -p "$(dirname "$1")"
    printf '#!/bin/sh\nexit 0\n' > "$1"
    chmod +x "$1"
}

# Each case runs in a subshell with its own HOME and PATH, and a provision_pkg
# stub that records whether it ran instead of downloading anything.
run_case() {
    _name="$1"
    _setup="$2"
    _want_managed="$3"
    _want_provisioned="$4"
    _want_bin_expr="$5"

    (
        HOME="$(mktemp -d)"
        export HOME
        fakepath="$HOME/fakepath"
        mkdir -p "$fakepath"
        # Keep the standard dirs so coreutils still resolve; neither holds a
        # formae, so only what a case installs is discoverable.
        PATH="$fakepath:/usr/bin:/bin"
        export PATH
        # Probe nothing real: the fixed locations are redirected under HOME.
        FORMAE_TEST_LOCATIONS="$HOME/opt/pel/bin/formae $HOME/usr/local/bin/formae $HOME/.local/bin/formae"
        export FORMAE_TEST_LOCATIONS
        managed="$HOME/.formae-ai/opt/bin/formae"
        provisioned=0

        # shellcheck disable=SC2317
        provision_pkg() {
            provisioned=1
            stub_bin "$managed"
            return 0
        }

        unset FORMAE_BIN FORMAE_BIN_MANAGED

        eval "$_setup"

        resolve_formae stable 2>/dev/null

        expect "$_name: FORMAE_BIN_MANAGED" "$FORMAE_BIN_MANAGED" "$_want_managed"
        expect "$_name: provisioned" "$provisioned" "$_want_provisioned"
        expect "$_name: FORMAE_BIN" "$FORMAE_BIN" "$(eval echo "$_want_bin_expr")"

        exit $failures
    ) || failures=$((failures + 1))
}

# The user's own formae on PATH is used as-is, and nothing is downloaded.
run_case "own install on PATH" \
    'stub_bin "$fakepath/formae"' \
    0 0 '$fakepath/formae'

# A known install location is found even when PATH omits it, which is what a
# harness launched from a desktop session looks like.
run_case "own install off PATH is still found" \
    'mkdir -p "$HOME/.formae-ai/opt/bin"' \
    1 1 '$managed'

# No formae anywhere: provision one into the user's own tree and own it.
run_case "clean machine provisions a managed copy" \
    'true' \
    1 1 '$managed'

# A managed copy already present is ours, and provision_pkg fast-paths on it.
run_case "existing managed copy stays managed" \
    'stub_bin "$managed"' \
    1 1 '$managed'

# The managed copy appearing on PATH must not be mistaken for a user install,
# or we would report it as needing sudo to upgrade.
run_case "managed copy on PATH is still managed" \
    'stub_bin "$managed"; PATH="$HOME/.formae-ai/opt/bin:$PATH"' \
    1 1 '$managed'

# A pre-set FORMAE_BIN is the testing escape hatch: honoured verbatim, nothing
# downloaded, and treated as the user's own so nothing upgrades it silently.
run_case "preset FORMAE_BIN wins outright" \
    'stub_bin "$HOME/custom/formae"; FORMAE_BIN="$HOME/custom/formae"' \
    0 0 '$HOME/custom/formae'

# Ownership is derived, never inherited. A stale FORMAE_BIN_MANAGED=1 in the
# environment must not make us treat someone else's binary as ours to upgrade:
# /formae:upgrade would then move the managed copy while the MCP kept running
# this one, and report success.
run_case "a stale FORMAE_BIN_MANAGED does not survive" \
    'stub_bin "$HOME/custom/formae"; FORMAE_BIN="$HOME/custom/formae"; FORMAE_BIN_MANAGED=1' \
    0 0 '$HOME/custom/formae'

# A user-local install is found even when the harness PATH omits it, which is
# what a desktop-launched session looks like. Missing it provisions a second
# formae alongside one that is already there.
run_case "user-local install off PATH is found" \
    'stub_bin "$HOME/.local/bin/formae"' \
    0 0 '$HOME/.local/bin/formae'

# A symlink into a probed location pointing back at the managed copy is still
# the managed copy. Compared as strings the two paths differ, and calling it the
# user's own would hand /formae:upgrade a binary it then refuses to touch.
run_case "a symlink to the managed copy is still managed" \
    'stub_bin "$managed"; mkdir -p "$HOME/.local/bin"; ln -s "$managed" "$HOME/.local/bin/formae"' \
    1 1 '$managed'

# FORMAE_BIN is the binary's own location, not the name it was reached by. The
# launcher puts the directory of FORMAE_BIN on PATH so formae can find the pkl
# beside it; a symlink's directory holds no pkl, and plugin manifests then fail
# to evaluate.
run_case "a symlinked own install resolves to its target" \
    'stub_bin "$HOME/real/formae"; ln -s "$HOME/real/formae" "$fakepath/formae"' \
    0 0 '$HOME/real/formae'

# The cases above inject their own probe list, so nothing there pins the list
# that actually ships. Check the defaults directly: dropping a location here is
# how a machine ends up with a second formae beside one it already had.
for loc in /opt/pel/bin/formae /usr/local/bin/formae '$HOME/.local/bin/formae' '$HOME/bin/formae'; do
    grep -q -- "$loc" "$ROOT/scripts/provision.sh" \
        || fail "the default probe list no longer includes $loc"
done

if [ "$failures" -ne 0 ]; then
    echo "resolve-formae: $failures check(s) failed" >&2
    exit 1
fi
echo "resolve-formae: all checks passed"
