#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BIN="$ROOT/bin/formae-mcp"
BUNDLED="$ROOT/bin/formae"

# Expose the bundled formae to the MCP (classic fallback + zero-setup path).
[ -x "$BUNDLED" ] && export FORMAE_BUNDLED_BIN="$BUNDLED"

newest_src="$(find "$ROOT/cmd" "$ROOT/internal" "$ROOT/go.mod" "$ROOT/go.sum" -type f -newer "$BIN" 2>/dev/null | head -n1 || true)"

# Dev fallback: rebuild only if a Go toolchain is present AND sources are newer
# (or the binary is missing). Otherwise run the shipped prebuilt binary as-is.
if { [ ! -x "$BIN" ] || [ -n "$newest_src" ]; } && command -v go >/dev/null 2>&1; then
	( cd "$ROOT" && go build -o "$BIN" ./cmd/formae-mcp )
fi

exec "$BIN" "$@"
