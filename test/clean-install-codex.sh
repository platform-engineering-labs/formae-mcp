#!/usr/bin/env sh
# clean-install-codex.sh — spin a clean container to test the Codex install
# flow from .codex/INSTALL.md with binaries pulled from a given channel.
#
# Follows the INSTALL doc steps verbatim: clone the repo, symlink the skills,
# register the MCP via `codex mcp add formae -- .../scripts/start-mcp.sh`. Then
# proves Codex actually spawns the launcher and the MCP connects by driving one
# non-interactive `codex exec` turn that calls the agentless `search_hub_plugins`
# tool — a full initialize + tools/list + tools/call round-trip through the
# harness. (`codex mcp list` only reports config state, never connectivity, so
# the tool call is the connect proof.)
#
# The agent-backed check (`check_health` against a live formae agent) is NOT
# covered here — it needs a reachable agent and a profile.
#
# Usage:  test/clean-install-codex.sh [channel] [ref]
#           channel  package channel to pull binaries from (default "dev")
#           ref      git ref of this repo to clone in the container
#                    (default: the current branch)
#         IMAGE=node:22-bookworm test/clean-install-codex.sh dev my-branch
#
# Requires: docker, a logged-in Codex on the host (~/.codex/auth.json — copied
# into the container so token refreshes never touch the host file), and the
# chosen ref pushed to origin (the repo must be reachable).
set -eu

CHANNEL="${1:-dev}"
REF="${2:-$(git rev-parse --abbrev-ref HEAD)}"
REPO="${REPO:-platform-engineering-labs/formae-mcp}"
IMAGE="${IMAGE:-node:22-bookworm}"

AUTH="$HOME/.codex/auth.json"
if [ ! -f "$AUTH" ]; then
    echo "error: $AUTH not found — run 'codex login' on the host first" >&2
    exit 1
fi

echo "Clean container (Codex): image=$IMAGE channel=$CHANNEL plugin=$REPO@$REF"

exec docker run --rm --network host \
  -e FORMAE_MCP_CHANNEL="$CHANNEL" -e REPO="$REPO" -e REF="$REF" \
  -v "$AUTH:/host-codex-auth.json:ro" \
  "$IMAGE" bash -lc '
    set -eu
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates git >/dev/null 2>&1
    echo "installing Codex CLI..."
    npm i -g @openai/codex >/dev/null 2>&1
    mkdir -p ~/.codex && cp /host-codex-auth.json ~/.codex/auth.json

    # No Go toolchain in the image — anything that lands in ~/.formae-ai/opt
    # was downloaded, not built.
    ! command -v go >/dev/null || { echo "FAIL: go toolchain present — image cannot prove download-not-build"; exit 1; }

    # --- .codex/INSTALL.md steps ---
    git clone -q https://github.com/$REPO.git ~/.codex/formae
    git -C ~/.codex/formae checkout -q "$REF"
    mkdir -p ~/.agents/skills
    ln -s ~/.codex/formae/skills ~/.agents/skills/formae
    # The INSTALL doc TOML block (required=true makes Codex wait for the server
    # at session start; without it the first run, which downloads the binaries,
    # silently has no formae tools). The env entry pins the test channel.
    cat > ~/.codex/config.toml <<TOML
[mcp_servers.formae]
command = "$HOME/.codex/formae/scripts/start-mcp.sh"
required = true
startup_timeout_sec = 120

[mcp_servers.formae.env]
FORMAE_MCP_CHANNEL = "$FORMAE_MCP_CHANNEL"
TOML

    echo "--- codex mcp list ---"
    codex mcp list
    codex mcp list --json | grep -q "\"formae\"" || { echo "FAIL: formae not in codex mcp list"; exit 1; }

    echo "--- codex exec: cold-spawn the MCP and call search_hub_plugins ---"
    cd ~
    codex exec --json --skip-git-repo-check \
      "Call the formae MCP tool search_hub_plugins with query \"aws\". You MUST invoke the tool. If it is not available, reply with exactly: TOOLS-MISSING" \
      </dev/null 2>/tmp/exec.err | tee /tmp/exec.jsonl || true

    grep "\"server\":\"formae\"" /tmp/exec.jsonl | grep "\"tool\":\"search_hub_plugins\"" | grep -q "\"status\":\"completed\"" \
      || { echo "FAIL: no completed formae search_hub_plugins call in codex exec output"; sed -n "1,40p" /tmp/exec.err; exit 1; }

    [ -x ~/.formae-ai/opt/bin/formae-mcp ] || { echo "FAIL: formae-mcp not provisioned"; exit 1; }
    [ -x ~/.formae-ai/opt/bin/formae ]     || { echo "FAIL: formae not provisioned"; exit 1; }

    echo
    echo "PASS: Codex spawned start-mcp.sh, binaries downloaded (no build, no sudo), formae tool call completed"
    ls -l ~/.formae-ai/opt/bin/
  '
