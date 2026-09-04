#!/usr/bin/env sh
# provision.sh — sourceable helper that installs a pel package into the
# dedicated user tree ~/.formae-ai/opt without sudo.
#
# Usage (source this file, then call the function):
#   . scripts/provision.sh
#   provision_pkg formae dev
#
# Environment:
#   FORMAE_FORCE_PROVISION  — if non-empty, reinstall even if the binary exists.

# provision_pkg <pkg> <channel>
#   Installs <pkg> from <channel> into ~/.formae-ai/opt using pelmgr.
#   The binary lands at ~/.formae-ai/opt/bin/<pkg>.
#   Returns non-zero on failure; progress is written to stderr.
provision_pkg() {
    _ppkg="$1"
    _pchan="$2"

    _tree="$HOME/.formae-ai/opt"
    _bin="$_tree/bin/$_ppkg"

    # Fast path: already installed and FORMAE_FORCE_PROVISION is not set.
    if [ -x "$_bin" ] && [ -z "${FORMAE_FORCE_PROVISION:-}" ]; then
        echo "provision_pkg: $_ppkg already at $_bin, skipping" >&2
        return 0
    fi

    # Orbital walks up only one directory level when deciding whether sudo is
    # needed. If ~/.formae-ai is missing it cannot find the existing tree root
    # and wrongly demands privilege escalation. Create the parent first.
    mkdir -p "$HOME/.formae-ai"

    if command -v pelmgr >/dev/null 2>&1; then
        # pelmgr is already on PATH — invoke it directly.
        echo "provision_pkg: installing $_ppkg (channel: $_pchan) via pelmgr" >&2
        pelmgr --install-path "$_tree" install --channel "$_pchan" --yes "$_ppkg"
    else
        # Bootstrap pelmgr via the hub setup script.
        # The `--` after the inline script string is bash's $0 placeholder, NOT
        # a flag separator passed to pelmgr. Using `-c "..." -- install ...`
        # means $0="--" inside the script and the remaining words become $@ of
        # bash itself; the setup.sh one-liner reads them as its own arguments.
        # Do NOT write `bash setup.sh -- install …` — that passes literal `--`
        # to pelmgr and breaks flag parsing.
        echo "provision_pkg: pelmgr not found, bootstrapping via hub setup.sh" >&2
        bash -c "$(curl -fsSL https://hub.platform.engineering/get/setup.sh)" \
            -- install \
            --install-path "$_tree" \
            --channel "$_pchan" \
            --yes \
            "$_ppkg"
    fi
}

# canonical_path <path>
#   Prints <path> with symlinks resolved, in the link itself and in every
#   directory leading to it.
#
#   Two spellings of the same binary have to compare equal or resolve_formae
#   disowns its own install: a link at ~/.local/bin/formae pointing into the
#   managed tree is not a second formae, and a home directory that is itself a
#   symlink makes even the managed path arrive spelled two ways. Either one sets
#   FORMAE_BIN_MANAGED=0, and /formae:upgrade then refuses to upgrade a binary
#   that was ours all along.
#
#   Done by hand because readlink -f and realpath are both shorter and neither
#   ships on a stock macOS. There is no loop guard: callers walk a path that
#   `test -x` has already accepted, and -x is false for a symlink cycle.
canonical_path() {
    _cppath="$1"
    while [ -L "$_cppath" ]; do
        _cplink="$(readlink -- "$_cppath")" || break
        case "$_cplink" in
            /*) _cppath="$_cplink" ;;
            *)  _cppath="$(dirname -- "$_cppath")/$_cplink" ;;
        esac
    done
    if _cpdir="$(CDPATH= cd -P -- "$(dirname -- "$_cppath")" 2>/dev/null && pwd -P)"; then
        printf '%s/%s\n' "$_cpdir" "$(basename -- "$_cppath")"
    else
        printf '%s\n' "$_cppath"
    fi
}

# resolve_formae <channel>
#   Decides which formae this machine runs, provisioning one only when it has
#   none. Sets FORMAE_BIN (absolute path) and FORMAE_BIN_MANAGED (1 when that
#   binary is ours to upgrade without sudo, else 0).
#
#   There is exactly one formae per machine. Detection lives here and only here:
#   the MCP reads FORMAE_BIN rather than repeating the search in Go, because two
#   detectors in two languages can disagree, and that disagreement is how a
#   machine ends up with two installs shadowing each other.
#
#   A pre-set FORMAE_BIN wins outright. That is the escape hatch for testing
#   against a specific build (pair it with FORMAE_MCP_CHANNEL=dev), and it counts
#   as the user's own install so nothing upgrades it behind their back.
resolve_formae() {
    _rchan="$1"
    _rmanaged="$HOME/.formae-ai/opt/bin/formae"
    _rmanagedreal="$(canonical_path "$_rmanaged")"
    # Test seam: the fixed locations probed after PATH. Overridden only by
    # test/resolve-formae.sh, so a machine that really has formae installed
    # cannot reach into the clean-machine cases.
    _rlocations="${FORMAE_TEST_LOCATIONS:-/opt/pel/bin/formae /usr/local/bin/formae $HOME/.local/bin/formae $HOME/bin/formae}"

    if [ -n "${FORMAE_BIN:-}" ]; then
        # Always the user's own: we did not install it, so we never move it.
        # Ownership is derived here rather than trusted from the environment,
        # so a stale FORMAE_BIN_MANAGED cannot make us upgrade someone else's
        # binary — or, worse, upgrade ours while running theirs.
        FORMAE_BIN_MANAGED=0
        return 0
    fi

    # The user's own install wins, wherever it is. The known locations are
    # probed after PATH because a harness launched from a desktop session can
    # have a minimal PATH that omits them.
    # shellcheck disable=SC2086 # _rlocations is a space-separated candidate list.
    for _rcand in "$(command -v formae 2>/dev/null || true)" $_rlocations; do
        [ -n "$_rcand" ] && [ -x "$_rcand" ] || continue
        # Resolved before the comparison, and kept resolved: FORMAE_BIN is the
        # binary's own location, not the name it was reached by. The launcher
        # puts the directory of FORMAE_BIN on PATH so formae finds the pkl
        # beside it, and a symlink's directory holds no pkl.
        _rcand="$(canonical_path "$_rcand")"
        if [ "$_rcand" != "$_rmanagedreal" ]; then
            FORMAE_BIN="$_rcand"
            FORMAE_BIN_MANAGED=0
            echo "resolve_formae: using your formae at $FORMAE_BIN" >&2
            return 0
        fi
    done

    # Nothing installed: lay one down in the user's own tree, sudo-free.
    provision_pkg formae "$_rchan" || return 1

    # And the auth plugin, because a formae we provisioned is one nobody else is
    # going to complete.
    #
    # formae's own package depends on pkl and nothing else, so provisioning it
    # alone leaves a tree with no plugins in it at all. The oidc plugin ships in
    # the `standard` bundle, but nothing on this path installs that bundle — so
    # for a user whose formae came from here, a hosted sign-in had no auth plugin
    # to drive and could never succeed.
    #
    # Just oidc, not `standard`: the bundle also carries the cloud resource
    # plugins, which are hundreds of megabytes the user has not asked for yet and
    # which nothing in onboarding needs. They install what their forma requires.
    #
    # Best-effort. A failure here leaves a working formae that cannot sign in to
    # the hosted platform, which is exactly what it could do before, and the
    # sign-in path names the remedy.
    provision_pkg oidc "$_rchan" || echo "provision_pkg: oidc not installed; a hosted sign-in will say so" >&2
    FORMAE_BIN="$_rmanaged"
    FORMAE_BIN_MANAGED=1
    return 0
}
