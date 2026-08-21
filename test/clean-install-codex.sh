#!/usr/bin/env sh
# clean-install-codex.sh — spin a clean container to test the Codex *plugin*
# install flow (.codex/INSTALL.md's primary path) with binaries pulled from a
# given channel.
#
# Codex >=0.148 installs plugins from Claude-format marketplaces: it reads
# .claude-plugin/marketplace.json and discovers the plugin manifest at
# .codex-plugin/plugin.json. This script builds a local marketplace pointed at
# a clone of this repo, installs the plugin the same way an end user would
# (`codex plugin marketplace add` + `codex plugin add`), and proves Codex
# actually spawns the launcher and the MCP connects by driving one
# non-interactive `codex exec` turn that calls the agentless `search_hub_plugins`
# tool — a full initialize + tools/list + tools/call round-trip through the
# harness. (`codex plugin list` only reports install state, never connectivity,
# so the tool call is the connect proof.)
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

echo "Clean container (Codex plugin): image=$IMAGE channel=$CHANNEL plugin=$REPO@$REF"

exec docker run --rm --network host \
  -e FORMAE_MCP_CHANNEL="$CHANNEL" -e REPO="$REPO" -e REF="$REF" \
  -v "$AUTH:/host-codex-auth.json:ro" \
  "$IMAGE" bash -lc '
    set -eu
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates git >/dev/null 2>&1
    echo "installing Codex CLI..."
    npm i -g @openai/codex >/dev/null 2>&1
    echo "codex version: $(codex --version)"
    mkdir -p ~/.codex && cp /host-codex-auth.json ~/.codex/auth.json

    # No Go toolchain in the image — anything that lands in ~/.formae-ai/opt
    # was downloaded, not built.
    ! command -v go >/dev/null || { echo "FAIL: go toolchain present — image cannot prove download-not-build"; exit 1; }

    # --- clone the plugin repo and pin the test channel ---
    CLONE="$HOME/src/formae-mcp"
    git clone -q "https://github.com/$REPO.git" "$CLONE"
    git -C "$CLONE" checkout -q "$REF"

    # Inject FORMAE_MCP_CHANNEL into the plugin-declared MCP config so the
    # launcher pulls binaries from the channel under test instead of stable
    # (mirrors how the other clean-install scripts pin the dev channel; prod
    # users get stable by default with no env override at all).
    node -e "
      const fs = require(\"fs\");
      const p = process.argv[1];
      const cfg = JSON.parse(fs.readFileSync(p, \"utf8\"));
      cfg.mcpServers.formae.env = { FORMAE_MCP_CHANNEL: process.env.FORMAE_MCP_CHANNEL };
      fs.writeFileSync(p, JSON.stringify(cfg, null, 2) + \"\n\");
    " "$CLONE/.codex-plugin/mcp.json"
    echo "--- .codex-plugin/mcp.json (channel-pinned) ---"
    cat "$CLONE/.codex-plugin/mcp.json"

    # --- build a local marketplace naming the plugin "formae", sourced from the clone ---
    MKT="$HOME/mkt"
    mkdir -p "$MKT/.claude-plugin"
    mv "$CLONE" "$MKT/formae"
    cat > "$MKT/.claude-plugin/marketplace.json" <<JSON
{
  "name": "formae-marketplace",
  "owner": { "name": "clean-install-test" },
  "metadata": { "description": "local test marketplace for the formae Codex plugin", "version": "0.0.0" },
  "plugins": [
    {
      "name": "formae",
      "source": "./formae",
      "description": "formae MCP + skills (clean-install test; pulls binaries from the chosen channel)"
    }
  ]
}
JSON
    echo "--- marketplace.json ---"
    cat "$MKT/.claude-plugin/marketplace.json"

    # --- install exactly as documented in .codex/INSTALL.md ---
    codex plugin marketplace add "$MKT"
    codex plugin add formae@formae-marketplace

    echo "--- codex plugin list ---"
    codex plugin list
    codex plugin list | grep -q "formae" || { echo "FAIL: formae not in codex plugin list"; exit 1; }

    echo "--- codex exec: cold-spawn the MCP and call search_hub_plugins ---"
    cd ~
    START=$(date +%s)
    codex exec --json --skip-git-repo-check \
      "Call the formae MCP tool search_hub_plugins with query \"aws\". You MUST invoke the tool. If it is not available, reply with exactly: TOOLS-MISSING" \
      </dev/null 2>/tmp/exec.err | tee /tmp/exec.jsonl || true
    END=$(date +%s)
    echo "cold codex exec wall-clock: $((END - START))s"

    grep "\"server\":\"formae\"" /tmp/exec.jsonl | grep "\"tool\":\"search_hub_plugins\"" | grep -q "\"status\":\"completed\"" \
      || { echo "FAIL: no completed formae search_hub_plugins call in codex exec output"; sed -n "1,40p" /tmp/exec.err; exit 1; }

    [ -x ~/.formae-ai/opt/bin/formae-mcp ] || { echo "FAIL: formae-mcp not provisioned"; exit 1; }
    [ -x ~/.formae-ai/opt/bin/formae ]     || { echo "FAIL: formae not provisioned"; exit 1; }

    echo
    echo "PASS: Codex installed the plugin from the marketplace, spawned start-mcp.sh,"
    echo "binaries downloaded (no build, no sudo), formae tool call completed"
    ls -l ~/.formae-ai/opt/bin/

    # --- optional: exercise the update flow (informs whether INSTALL.md can
    # document `marketplace upgrade` as a working path, or only remove+add) ---
    echo
    echo "--- exercising update flow (marketplace upgrade, remove, re-add) ---"
    UPDATE_OK=1
    codex plugin marketplace upgrade || { echo "update flow: marketplace upgrade failed"; UPDATE_OK=0; }
    codex plugin remove formae@formae-marketplace || { echo "update flow: remove failed"; UPDATE_OK=0; }
    codex plugin add formae@formae-marketplace || { echo "update flow: re-add failed"; UPDATE_OK=0; }
    codex plugin list | grep -q "formae" || { echo "update flow: formae missing from plugin list after re-add"; UPDATE_OK=0; }

    if [ "$UPDATE_OK" = "1" ]; then
        echo "--- codex exec: warm probe after update flow ---"
        codex exec --json --skip-git-repo-check \
          "Call the formae MCP tool search_hub_plugins with query \"aws\". You MUST invoke the tool. If it is not available, reply with exactly: TOOLS-MISSING" \
          </dev/null 2>/tmp/exec2.err | tee /tmp/exec2.jsonl || true
        grep "\"server\":\"formae\"" /tmp/exec2.jsonl | grep "\"tool\":\"search_hub_plugins\"" | grep -q "\"status\":\"completed\"" \
          || { echo "update flow: warm probe did not complete a formae tool call"; UPDATE_OK=0; }
    fi

    if [ "$UPDATE_OK" = "1" ]; then
        echo "UPDATE-FLOW: PASS (marketplace upgrade + remove/add works end to end)"
    else
        echo "UPDATE-FLOW: FAIL (see messages above — document remove+re-add only)"
    fi
  '
