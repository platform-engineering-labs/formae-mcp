#!/usr/bin/env bash
set -euo pipefail

PLUGIN_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BINARY="${PLUGIN_ROOT}/bin/formae-mcp"

needs_rebuild() {
  # Rebuild if the binary doesn't exist yet.
  if [ ! -x "$BINARY" ]; then
    return 0
  fi
  # Rebuild if any tracked source is newer than the binary (e.g., after a
  # marketplace update fetches new files). Limit the search to inputs the
  # build actually consumes.
  if find \
    "${PLUGIN_ROOT}/cmd" \
    "${PLUGIN_ROOT}/internal" \
    "${PLUGIN_ROOT}/go.mod" \
    "${PLUGIN_ROOT}/go.sum" \
    -newer "$BINARY" -print -quit 2>/dev/null | grep -q .; then
    return 0
  fi
  return 1
}

if needs_rebuild; then
  echo "Building formae-mcp..." >&2
  mkdir -p "${PLUGIN_ROOT}/bin"
  (cd "$PLUGIN_ROOT" && go build -o "$BINARY" ./cmd/formae-mcp/)
  echo "Build complete." >&2
fi

exec "$BINARY" "$@"
