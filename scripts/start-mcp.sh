#!/usr/bin/env bash
set -euo pipefail

PLUGIN_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BINARY="${PLUGIN_ROOT}/bin/formae-mcp"

if [ ! -x "$BINARY" ]; then
  echo "Building formae-mcp (first run only)..." >&2
  mkdir -p "${PLUGIN_ROOT}/bin"
  (cd "$PLUGIN_ROOT" && go build -o "$BINARY" ./cmd/formae-mcp/)
  echo "Build complete." >&2
fi

exec "$BINARY" "$@"
