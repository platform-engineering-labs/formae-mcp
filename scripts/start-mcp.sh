#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

channel="${FORMAE_MCP_CHANNEL:-stable}"

. "$ROOT/scripts/provision.sh"

# Dev escape hatch: build from the worktree and exec immediately. formae is
# still resolved, so a dev launch shells out to the same binary a real one would.
if [ -n "${FORMAE_MCP_DEV:-}" ]; then
    echo "start-mcp: FORMAE_MCP_DEV set — building from source" >&2
    ( cd "$ROOT" && go build -o "$ROOT/bin/formae-mcp" ./cmd/formae-mcp )
    resolve_formae "$channel"
    export FORMAE_BIN FORMAE_BIN_MANAGED
    exec "$ROOT/bin/formae-mcp" "$@"
fi

# Plugin-version marker: if the plugin version has changed since the last
# launch, force a refresh of the MCP binary so an updated plugin release is
# not blocked by the existence fast-path in provision_pkg.
# The marker is intentionally NOT used for formae itself — that binary is only
# upgraded via an explicit /formae:upgrade invocation (pinned-CLI policy).
_plugin_version="$(grep -m1 '"version"' "$ROOT/.claude-plugin/plugin.json" \
    | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
_marker="$HOME/.formae-ai/opt/.formae-mcp.plugin-version"
_stored_version="$(cat "$_marker" 2>/dev/null || true)"

if [ -z "$_stored_version" ] || [ "$_stored_version" != "$_plugin_version" ]; then
    echo "start-mcp: plugin version changed ($( [ -z "$_stored_version" ] && echo "first run" || echo "$_stored_version -> $_plugin_version")), refreshing formae-mcp" >&2
    FORMAE_FORCE_PROVISION=1 provision_pkg formae-mcp "$channel"
    # Write marker; guard so a write failure does not abort the launch.
    mkdir -p "$(dirname "$_marker")" 2>/dev/null || true
    printf '%s' "$_plugin_version" > "$_marker" 2>/dev/null || true
else
    provision_pkg formae-mcp "$channel"
fi

FORMAE_MCP_BIN="$HOME/.formae-ai/opt/bin/formae-mcp"
if [ ! -x "$FORMAE_MCP_BIN" ]; then
    echo "start-mcp: provisioning succeeded but $FORMAE_MCP_BIN is not executable — aborting" >&2
    exit 1
fi

resolve_formae "$channel"
export FORMAE_BIN FORMAE_BIN_MANAGED

exec "$FORMAE_MCP_BIN" "$@"
