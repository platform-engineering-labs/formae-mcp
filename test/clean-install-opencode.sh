#!/usr/bin/env sh
# clean-install-opencode.sh — spin a clean container to test the OpenCode
# install flow from .opencode/INSTALL.md with binaries pulled from a given
# channel.
#
# Follows the INSTALL doc steps verbatim: clone the repo, symlink the skills,
# register the MCP in ~/.config/opencode/opencode.json pointing at
# scripts/start-mcp.sh. `opencode mcp list` then spawns the server and reports
# its status, so a "formae ... connected" line proves the harness launched the
# script and completed the MCP handshake — no model call or provider auth
# needed. A direct JSON-RPC tools/list through the launcher then confirms the
# tool surface is served.
#
# The agent-backed check (`check_health` against a live formae agent) is NOT
# covered here — it needs a reachable agent and a profile.
#
# Usage:  test/clean-install-opencode.sh [channel] [ref]
#           channel  package channel to pull binaries from (default "dev")
#           ref      git ref of this repo to clone in the container
#                    (default: the current branch)
#         IMAGE=node:22-bookworm test/clean-install-opencode.sh dev my-branch
#
# Requires: docker and the chosen ref pushed to origin (the repo must be
# reachable).
set -eu

CHANNEL="${1:-dev}"
REF="${2:-$(git rev-parse --abbrev-ref HEAD)}"
REPO="${REPO:-platform-engineering-labs/formae-mcp}"
IMAGE="${IMAGE:-node:22-bookworm}"

echo "Clean container (OpenCode): image=$IMAGE channel=$CHANNEL plugin=$REPO@$REF"

exec docker run --rm --network host \
  -e FORMAE_MCP_CHANNEL="$CHANNEL" -e REPO="$REPO" -e REF="$REF" \
  "$IMAGE" bash -lc '
    set -eu
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates git >/dev/null 2>&1
    echo "installing OpenCode..."
    npm i -g opencode-ai >/dev/null 2>&1

    # No Go toolchain in the image — anything that lands in ~/.formae-ai/opt
    # was downloaded, not built.
    ! command -v go >/dev/null || { echo "FAIL: go toolchain present — image cannot prove download-not-build"; exit 1; }

    # --- .opencode/INSTALL.md steps ---
    git clone -q https://github.com/$REPO.git ~/.config/opencode/formae
    git -C ~/.config/opencode/formae checkout -q "$REF"
    mkdir -p ~/.config/opencode/skills
    ln -s ~/.config/opencode/formae/skills ~/.config/opencode/skills/formae
    cat > ~/.config/opencode/opencode.json <<JSON
{
  "\$schema": "https://opencode.ai/config.json",
  "mcp": {
    "formae": {
      "type": "local",
      "command": ["$HOME/.config/opencode/formae/scripts/start-mcp.sh"],
      "environment": { "FORMAE_MCP_CHANNEL": "$FORMAE_MCP_CHANNEL" },
      "enabled": true
    }
  }
}
JSON

    echo "--- opencode mcp list (cold spawn: downloads the binaries first) ---"
    cd ~
    opencode mcp list 2>&1 | tee /tmp/mcp-list.out
    grep -q "formae" /tmp/mcp-list.out || { echo "FAIL: formae not in opencode mcp list"; exit 1; }
    grep -q "connected" /tmp/mcp-list.out || { echo "FAIL: formae not connected"; exit 1; }

    [ -x ~/.formae-ai/opt/bin/formae-mcp ] || { echo "FAIL: formae-mcp not provisioned"; exit 1; }
    [ -x ~/.formae-ai/opt/bin/formae ]     || { echo "FAIL: formae not provisioned"; exit 1; }

    echo "--- direct tools/list through the launcher (warm path) ---"
    # Keep stdin open after sending so the server can reply before EOF shuts it
    # down; it exits nonzero on the close, so the reply content is the check.
    { printf "%s\n%s\n%s\n" \
      "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-03-26\",\"capabilities\":{},\"clientInfo\":{\"name\":\"clean-install-test\",\"version\":\"0\"}}}" \
      "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}" \
      "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}"; sleep 3; } \
      | ~/.config/opencode/formae/scripts/start-mcp.sh > /tmp/tools.json 2>/dev/null || true
    grep -q "\"tools\"" /tmp/tools.json || { echo "FAIL: tools/list returned no tools"; exit 1; }
    echo "tools served: $(grep -o "\"name\"" /tmp/tools.json | wc -l)"

    echo
    echo "PASS: OpenCode spawned start-mcp.sh, binaries downloaded (no build, no sudo), MCP connected"
    ls -l ~/.formae-ai/opt/bin/
  '
