#!/usr/bin/env bash
# hosted-onboarding-demo.sh — one command to try the hosted onboarding journey in a
# clean container, from nothing.
#
# Share this file as-is. It needs only docker and git: formae is built inside a
# container, so no Go toolchain on the host.
#
#   ./hosted-onboarding-demo.sh
#
# What it does:
#   1. clones (or pulls) formae-mcp and formae at the branches under test
#   2. builds a formae carrying `formae login --hosted` — merged, but not yet in a
#      released build, so it has to come from source
#   3. launches test/clean-install.sh, which builds a clean container, installs
#      Claude Code, and drops you into a session
#
# Then, inside the session:
#   /login                          (unless you set CLAUDE_AUTH=copy)
#   /plugin marketplace add /mkt
#   /plugin install formae@formae-dev
#   /reload-plugins
#   /formae:setup
#
# Tell it you have no browser when it asks: the container's sign-in callback is on
# its own loopback, so a browser on your machine cannot complete it. The device
# code works from any device.
#
# Environment:
#   WORKDIR=<path>       where the repos are cloned  (default ~/formae-hosted-test)
#   CLAUDE_AUTH=copy     reuse your host Claude Code login instead of /login inside
#   CHANNEL=<name>       package channel             (default dev)
#   SKIP_PULL=1          do not fetch; use what is already on disk
set -euo pipefail

WORKDIR="${WORKDIR:-$HOME/formae-hosted-test}"
CHANNEL="${CHANNEL:-dev}"
CLAUDE_AUTH="${CLAUDE_AUTH:-none}"
GO_IMAGE="${GO_IMAGE:-golang:1.26}"

# Where each half lives.
#
# The formae half is merged, so it comes from main. The MCP half is not, so it
# comes from its branch — and that branch is also what the generated marketplace
# installs the plugin from, which is the point of the exercise.
MCP_REPO="https://github.com/platform-engineering-labs/formae-mcp.git"
MCP_REF="${MCP_REF:-mcp-onboarding}"
FORMAE_REPO="https://github.com/platform-engineering-labs/formae.git"
FORMAE_REF="${FORMAE_REF:-main}"

# The version stamp is not optional. The MCP refuses to talk to a formae below
# 0.89.0, and an unstamped build reports 0.0.0 — every tool then fails with
# "requires formae >= 0.89.0", which reads like a broken version gate rather than a
# missing build flag.
FORMAE_VERSION="${FORMAE_VERSION:-0.89.0}"

die() { printf '\n%s\n' "$*" >&2; exit 1; }
step() { printf '\n==> %s\n' "$*"; }

command -v docker >/dev/null 2>&1 || die "docker is required and was not found on PATH."
command -v git    >/dev/null 2>&1 || die "git is required and was not found on PATH."
docker info >/dev/null 2>&1 || die "docker is installed but not usable — is the daemon running, and are you in the docker group?"

# clean-install.sh word-splits its mount paths, so whitespace anywhere in them
# becomes two docker arguments and the container fails to start.
case "$WORKDIR$HOME" in
  *[[:space:]]*) die "Paths with spaces are not supported. Set WORKDIR to a path without spaces." ;;
esac

# sync <dir> <repo> <ref> — clone at ref, or fetch and hard-reset an existing copy.
# Reset rather than pull: these are throwaway checkouts of someone else's branches,
# and a merge conflict in a demo script is a worse outcome than discarding local
# edits nobody meant to make.
sync() {
  local dir="$1" repo="$2" ref="$3"
  if [ -d "$dir/.git" ]; then
    if [ -n "${SKIP_PULL:-}" ]; then
      step "using the existing checkout at $dir"
      return 0
    fi
    step "updating $(basename "$dir") to $ref"
    git -C "$dir" fetch --quiet origin "$ref"
    git -C "$dir" checkout --quiet -B "$ref" "origin/$ref"
    git -C "$dir" reset --hard --quiet "origin/$ref"
  else
    step "cloning $(basename "$dir") at $ref"
    git clone --quiet --branch "$ref" --single-branch "$repo" "$dir"
  fi
  printf '    %s\n' "$(git -C "$dir" log --oneline -1)"
}

mkdir -p "$WORKDIR"
sync "$WORKDIR/formae-mcp" "$MCP_REPO"    "$MCP_REF"
sync "$WORKDIR/formae"     "$FORMAE_REPO" "$FORMAE_REF"

# Built in a container so the host needs no Go, and as the invoking user so the
# binary is not left root-owned. The caches are redirected because the container
# user has no home directory to put them in.
step "building formae $FORMAE_VERSION (in $GO_IMAGE, so no Go is needed here)"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod -e HOME=/tmp \
  -v "$WORKDIR/formae:/src" -w /src \
  "$GO_IMAGE" \
  go build -ldflags="-X 'github.com/platform-engineering-labs/formae.Version=$FORMAE_VERSION'" \
    -o /src/formae-hosted ./cmd/formae

FORMAE_BIN="$WORKDIR/formae/formae-hosted"
[ -x "$FORMAE_BIN" ] || die "the formae build produced no binary at $FORMAE_BIN"
printf '    %s\n' "$("$FORMAE_BIN" --version 2>&1 | head -1)"

step "starting the container"
cat <<BANNER

  Inside the session:
BANNER
[ "$CLAUDE_AUTH" = "none" ] && printf '    /login\n'
cat <<'BANNER'
    /plugin marketplace add /mkt
    /plugin install formae@formae-dev
    /reload-plugins
    /formae:setup

  Say you have no browser when asked — the container's callback is on its own
  loopback, so the device code is the flow that works.

BANNER

cd "$WORKDIR/formae-mcp"
exec env \
  HOSTED=1 \
  DEV_BUILD=1 \
  CLAUDE_AUTH="$CLAUDE_AUTH" \
  FORMAE_LOCAL_BIN="$FORMAE_BIN" \
  test/clean-install.sh "$CHANNEL" "$MCP_REF"
