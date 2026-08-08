#!/usr/bin/env sh
# clean-install.sh — spin a clean container to test the formae plugin's
# marketplace install with binaries pulled from a given channel.
#
# It generates a local marketplace manifest whose single plugin is fetched from
# this repo on GitHub at a chosen ref (default: the current branch), mounts your
# host Claude Code credentials for auth, installs Claude Code, and drops you into
# an interactive session. Inside, run:
#
#   /plugin marketplace add /mkt
#   /plugin install formae@formae-dev
#   /reload-plugins
#   /mcp                 # the formae MCP should connect (binary pulled, no build, no sudo)
#   /formae:formae-status
#
# Usage:  test/clean-install.sh [channel] [ref]
#           channel  package channel to pull binaries from (default "dev")
#           ref      git ref of this repo to install the plugin from
#                    (default: the current branch)
#         IMAGE=node:22-bookworm test/clean-install.sh dev my-branch
#
# Requires: docker, a logged-in Claude Code on the host (~/.claude), and the
# chosen ref pushed to origin (the repo must be reachable — public, or auth'd).
set -eu

CHANNEL="${1:-dev}"
REF="${2:-$(git rev-parse --abbrev-ref HEAD)}"
REPO="${REPO:-platform-engineering-labs/formae-mcp}"
IMAGE="${IMAGE:-node:22-bookworm}"

# A local marketplace file whose plugin is a github source pinned to REF.
# (Marketplace "local" sources are plain relative-path strings, not this — the
# github source is both valid across Claude Code versions and prod-faithful.)
MKT="$(mktemp -d)"
mkdir -p "$MKT/.claude-plugin"
cat > "$MKT/.claude-plugin/marketplace.json" <<JSON
{
  "name": "formae-dev",
  "owner": { "name": "dev-local" },
  "metadata": { "description": "local dev marketplace for formae plugin testing", "version": "0.0.0" },
  "plugins": [
    {
      "name": "formae",
      "source": { "source": "github", "repo": "$REPO", "ref": "$REF" },
      "description": "formae MCP + skills (dev build; pulls binaries from the chosen channel)"
    }
  ]
}
JSON

# Optional host mounts: Claude Code auth + config, and formae profiles for
# check_health. Only mount what exists so the run never fails on a missing path.
MOUNTS=""
[ -e "$HOME/.claude" ]        && MOUNTS="$MOUNTS -v $HOME/.claude:/root/.claude"
[ -e "$HOME/.claude.json" ]   && MOUNTS="$MOUNTS -v $HOME/.claude.json:/root/.claude.json"
[ -e "$HOME/.config/formae" ] && MOUNTS="$MOUNTS -v $HOME/.config/formae:/root/.config/formae:ro"

echo "Clean container: image=$IMAGE channel=$CHANNEL plugin=$REPO@$REF"
echo "Inside the session, run:"
echo "  /plugin marketplace add /mkt"
echo "  /plugin install formae@formae-dev"
echo "  /reload-plugins ; /mcp ; /formae:formae-status"
echo

# shellcheck disable=SC2086  # intentional word-splitting of $MOUNTS
exec docker run -it --rm --network host \
  -e FORMAE_MCP_CHANNEL="$CHANNEL" \
  $MOUNTS \
  -v "$MKT:/mkt:ro" \
  "$IMAGE" bash -lc '
    set -e
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates git openssh-client >/dev/null 2>&1
    # Claude Code clones github plugin sources over SSH (git@github.com:...). A
    # fresh container has no known_hosts, so strict host-key checking aborts.
    # This repo is public, so force github clones over HTTPS (no auth, no SSH),
    # and seed known_hosts as a fallback for any SSH path that remains.
    git config --global url."https://github.com/".insteadOf "git@github.com:"
    git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"
    mkdir -p ~/.ssh && ssh-keyscan -t rsa,ecdsa,ed25519 github.com >> ~/.ssh/known_hosts 2>/dev/null || true
    echo "installing Claude Code..."
    npm i -g @anthropic-ai/claude-code >/dev/null 2>&1
    echo "ready — marketplace at /mkt, channel='"$CHANNEL"'"
    echo "run:  /plugin marketplace add /mkt   then   /plugin install formae@formae-dev"
    exec claude
  '
