#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

# Dev escape hatch: build from the worktree and exec immediately.
# formae resolves via the user's own install (FORMAE_BUNDLED_BIN is intentionally
# NOT set here so the dev environment uses the system formae).
if [ -n "${FORMAE_MCP_DEV:-}" ]; then
    echo "start-mcp: FORMAE_MCP_DEV set — building from source" >&2
    ( cd "$ROOT" && go build -o "$ROOT/bin/formae-mcp" ./cmd/formae-mcp )
    exec "$ROOT/bin/formae-mcp" "$@"
fi

# Normal path: download prebuilt binaries into the dedicated user tree.
channel="${FORMAE_MCP_CHANNEL:-stable}"

. "$ROOT/scripts/provision.sh"

provision_pkg formae-mcp "$channel"
provision_pkg formae "$channel"

FORMAE_MCP_BIN="$HOME/.formae-ai/opt/bin/formae-mcp"
if [ ! -x "$FORMAE_MCP_BIN" ]; then
    echo "start-mcp: provisioning succeeded but $FORMAE_MCP_BIN is not executable — aborting" >&2
    exit 1
fi

export FORMAE_BUNDLED_BIN="$HOME/.formae-ai/opt/bin/formae"

exec "$FORMAE_MCP_BIN" "$@"
