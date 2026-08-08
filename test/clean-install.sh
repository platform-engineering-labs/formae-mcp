#!/usr/bin/env sh
# clean-install.sh — spin a clean container to test the formae plugin's
# marketplace install with binaries pulled from a given channel.
#
# It mounts the plugin (this repo) read-only, generates a local marketplace
# manifest whose plugin source points at the in-container path, mounts your
# host Claude Code credentials for auth, installs Claude Code, and drops you
# into an interactive session. Inside, run:
#
#   /plugin marketplace add /mkt
#   /plugin install formae@formae-dev
#   /reload-plugins
#   /mcp                 # the formae MCP should connect (binary pulled, no build, no sudo)
#   /formae:formae-status
#
# Usage:  test/clean-install.sh [channel]        (channel defaults to "dev")
#         IMAGE=node:22-bookworm test/clean-install.sh dev
#
# Requires: docker, and a logged-in Claude Code on the host (~/.claude).
set -eu

CHANNEL="${1:-dev}"
IMAGE="${IMAGE:-node:22-bookworm}"
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"   # the plugin/repo dir

# Marketplace manifest whose plugin source path is the CONTAINER mount (/plugin),
# not the host path — otherwise `/plugin install` can't find the plugin inside.
MKT="$(mktemp -d)"
mkdir -p "$MKT/.claude-plugin"
cat > "$MKT/.claude-plugin/marketplace.json" <<'JSON'
{
  "name": "formae-dev",
  "owner": { "name": "dev-local" },
  "metadata": { "description": "local dev marketplace for formae plugin testing", "version": "0.0.0" },
  "plugins": [
    {
      "name": "formae",
      "source": { "source": "local", "path": "/plugin" },
      "description": "formae MCP + skills (dev build; pulls binaries from the chosen channel)"
    }
  ]
}
JSON

# Optional host mounts: Claude Code auth + config, and formae profiles for
# check_health. Only mount what exists so the run never fails on a missing path.
MOUNTS=""
[ -e "$HOME/.claude" ]         && MOUNTS="$MOUNTS -v $HOME/.claude:/root/.claude"
[ -e "$HOME/.claude.json" ]    && MOUNTS="$MOUNTS -v $HOME/.claude.json:/root/.claude.json"
[ -e "$HOME/.config/formae" ]  && MOUNTS="$MOUNTS -v $HOME/.config/formae:/root/.config/formae:ro"

echo "Clean container: image=$IMAGE channel=$CHANNEL plugin=$ROOT"
echo "Inside the session, run:"
echo "  /plugin marketplace add /mkt"
echo "  /plugin install formae@formae-dev"
echo "  /reload-plugins ; /mcp ; /formae:formae-status"
echo

# shellcheck disable=SC2086  # intentional word-splitting of $MOUNTS
exec docker run -it --rm --network host \
  -e FORMAE_MCP_CHANNEL="$CHANNEL" \
  $MOUNTS \
  -v "$ROOT:/plugin:ro" \
  -v "$MKT:/mkt:ro" \
  "$IMAGE" bash -lc '
    set -e
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates >/dev/null 2>&1
    echo "installing Claude Code..."
    npm i -g @anthropic-ai/claude-code >/dev/null 2>&1
    echo "ready — marketplace at /mkt, plugin at /plugin, channel='"$CHANNEL"'"
    echo "run:  /plugin marketplace add /mkt   then   /plugin install formae@formae-dev"
    exec claude
  '
