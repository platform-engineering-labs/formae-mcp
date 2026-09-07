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
