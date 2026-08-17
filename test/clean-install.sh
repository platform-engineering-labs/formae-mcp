#!/usr/bin/env sh
# clean-install.sh — spin a clean container to test the formae plugin's
# marketplace install with binaries pulled from a given channel.
#
# It generates a local marketplace manifest whose single plugin is fetched from
# this repo on GitHub at a chosen ref (default: the current branch), mounts your
# host Claude Code credentials for auth, installs Claude Code, and drops you into
# an interactive session. Inside, run:
#
#   /plugin marketplace add /mkt
#   /plugin install formae@formae-dev
#   /reload-plugins
#   /mcp                 # the formae MCP should connect (binary pulled, no build, no sudo)
#   /formae:formae-status
#
# Usage:  test/clean-install.sh [channel] [ref]
#           channel  package channel to pull binaries from (default "dev")
#           ref      git ref of this repo to install the plugin from
#                    (default: the current branch)
#         IMAGE=node:22-bookworm test/clean-install.sh dev my-branch
#
# Env:
#   DEV_BUILD=1       install Go and set FORMAE_MCP_DEV=1, so the MCP is built
#                     from the plugin checkout instead of pulled from the
#                     channel. Use this to test branch code: the channel only
#                     ever carries released binaries, so without it the scripts
#                     and skills come from REF while the binary does not.
#   MOUNT_PROFILES=0  do not mount the host ~/.config/formae, so the container
#                     is a machine where formae has never run. Required to
#                     exercise first-run: bootstrap, provisioning, and setup.
#   CLAUDE_AUTH       where Claude Code's own auth comes from. Default "none".
#                       none  nothing is mounted — a genuinely fresh machine, so
#                             you run /login inside, as a new user does. Slower by
#                             one sign-in, and the only faithful option.
#                       copy  a throwaway copy of ~/.claude, so the container is
#                             authenticated without a plugin install landing in
#                             the host's own config. Convenient, not faithful.
#                       host  the real ~/.claude, read-write. Fastest, and an
#                             install inside the container lands on the host too.
#                     ISOLATE_CLAUDE=1 is still accepted and means "copy".
#   HOSTED=1          set up for the hosted onboarding journey: no host profiles
#                     (so the machine really is unconfigured), the control-plane
#                     pair exported, and the oidc auth plugin installed — nothing
#                     else installs it, since formae's own package requires only
#                     pkl. Implies MOUNT_PROFILES=0.
#   FORMAE_LOCAL_BIN  path to a locally built formae to mount instead of pulling
#                     one from the channel. Needed for hosted testing until a dev
#                     build carrying `formae login --hosted` is published; see
#                     the note under HOSTED below.
#   CLOUD_URL         control plane origin   (default https://console.formae.ai)
#   CLOUD_ISSUER      control plane issuer   (default https://auth.formae.ai)
#                     Both or neither: formae refuses a half-set pair, because a
#                     custom control plane paired with the default issuer is the
#                     state its credential gate exists to catch.
#
# On HOSTED=1 and what it can show you:
#
#   Without FORMAE_LOCAL_BIN this exercises the real distribution path — the
#   marketplace install, the formae-mcp download, and the formae download — and
#   then stops at `unknown flag: --hosted`, because the channel only carries
#   released binaries and hosted sign-in is not released yet. That is the correct
#   outcome, not a broken script.
#
#   With FORMAE_LOCAL_BIN the hosted journey completes, and formae is NOT
#   downloaded: a binary at one of the probed locations is "your own install",
#   which is exactly what stops the plugin replacing it. The formae-mcp download
#   still happens. The two cannot both be demonstrated in one run.
#
# Requires: docker, and the chosen ref pushed to origin (the repo must be
# reachable — public, or auth'd). A logged-in Claude Code on the host is needed
# only for CLAUDE_AUTH=copy/host; the default signs in inside the container.
set -eu

CHANNEL="${1:-dev}"
REF="${2:-$(git rev-parse --abbrev-ref HEAD)}"
REPO="${REPO:-platform-engineering-labs/formae-mcp}"
IMAGE="${IMAGE:-node:22-bookworm}"
DEV_BUILD="${DEV_BUILD:-0}"
MOUNT_PROFILES="${MOUNT_PROFILES:-1}"
ISOLATE_CLAUDE="${ISOLATE_CLAUDE:-0}"
# A fresh machine is the default, because that is what the thing under test is for:
# a new user has no ~/.claude to mount, and mounting ours hides whatever a real
# first run would hit. ISOLATE_CLAUDE=1 still selects the copy.
CLAUDE_AUTH="${CLAUDE_AUTH:-none}"
[ "$ISOLATE_CLAUDE" = "1" ] && CLAUDE_AUTH="copy"
GO_VERSION="${GO_VERSION:-1.25.1}"
HOSTED="${HOSTED:-0}"
FORMAE_LOCAL_BIN="${FORMAE_LOCAL_BIN:-}"
CLOUD_URL="${CLOUD_URL:-https://console.formae.ai}"
CLOUD_ISSUER="${CLOUD_ISSUER:-https://auth.formae.ai}"

# The onboarding journey starts from a machine with nothing configured, so the
# host's profiles must not be mounted: with them, the gate never fires and setup
# has nothing to set up.
if [ "$HOSTED" = "1" ]; then
    MOUNT_PROFILES=0
fi

# A local marketplace file whose plugin is a github source pinned to REF.
# (Marketplace "local" sources are plain relative-path strings, not this — the
# github source is both valid across Claude Code versions and prod-faithful.)
MKT="$(mktemp -d)"
mkdir -p "$MKT/.claude-plugin"
cat > "$MKT/.claude-plugin/marketplace.json" <<JSON
{
  "name": "formae-dev",
  "owner": { "name": "dev-local" },
  "metadata": { "description": "local dev marketplace for formae plugin testing", "version": "0.0.0" },
  "plugins": [
    {
      "name": "formae",
      "source": { "source": "url", "url": "https://github.com/$REPO.git", "ref": "$REF" },
      "description": "formae MCP + skills (dev build; pulls binaries from the chosen channel)"
    }
  ]
}
JSON

# Optional host mounts: Claude Code auth + config, and formae profiles for
# check_health. Only mount what exists so the run never fails on a missing path.
MOUNTS=""
case "$CLAUDE_AUTH" in
    none)
        # Nothing mounted. The container has no Claude Code config at all, which
        # is the state a new user is in.
        ;;
    copy)
        CLAUDE_HOME="$(mktemp -d)"
        [ -e "$HOME/.claude" ]      && cp -a "$HOME/.claude" "$CLAUDE_HOME/.claude"
        [ -e "$HOME/.claude.json" ] && cp -a "$HOME/.claude.json" "$CLAUDE_HOME/.claude.json"
        echo "Claude config: throwaway copy at $CLAUDE_HOME"
        [ -e "$CLAUDE_HOME/.claude" ]      && MOUNTS="$MOUNTS -v $CLAUDE_HOME/.claude:/root/.claude"
        [ -e "$CLAUDE_HOME/.claude.json" ] && MOUNTS="$MOUNTS -v $CLAUDE_HOME/.claude.json:/root/.claude.json"
        ;;
    host)
        [ -e "$HOME/.claude" ]      && MOUNTS="$MOUNTS -v $HOME/.claude:/root/.claude"
        [ -e "$HOME/.claude.json" ] && MOUNTS="$MOUNTS -v $HOME/.claude.json:/root/.claude.json"
        ;;
    *)
        echo "CLAUDE_AUTH must be none, copy, or host (got '$CLAUDE_AUTH')" >&2
        exit 1
        ;;
esac
if [ "$MOUNT_PROFILES" = "1" ] && [ -e "$HOME/.config/formae" ]; then
    MOUNTS="$MOUNTS -v $HOME/.config/formae:/root/.config/formae:ro"
fi

echo "Clean container: image=$IMAGE channel=$CHANNEL plugin=$REPO@$REF"
if [ "$DEV_BUILD" = "1" ]; then
    echo "  MCP: built from the plugin checkout (FORMAE_MCP_DEV=1, go $GO_VERSION)"
else
    echo "  MCP: prebuilt from the '$CHANNEL' channel — branch Go changes will NOT be present"
fi
if [ "$MOUNT_PROFILES" = "1" ]; then
    echo "  profiles: host ~/.config/formae mounted read-only"
else
    echo "  profiles: none — a machine where formae has never run"
fi
case "$CLAUDE_AUTH" in
    none) echo "  claude auth:   none — a fresh machine; run /login inside, as a new user does" ;;
    copy) echo "  claude auth:   throwaway copy — host plugin set untouched" ;;
    host) echo "  claude auth:   host ~/.claude read-write — installs land on the host too" ;;
esac
if [ "$HOSTED" = "1" ]; then
    echo "  hosted:   control plane $CLOUD_URL (issuer $CLOUD_ISSUER)"
    if [ -n "$FORMAE_LOCAL_BIN" ]; then
        echo "  formae:   mounted from $FORMAE_LOCAL_BIN — NOT downloaded (a local binary is 'your own install')"
        echo "            oidc + pkl are laid down for it here, since a mount skips provisioning"
    else
        echo "  formae:   pulled from the '$CHANNEL' channel, with oidc alongside it."
        echo "            NOTE: released binaries have no 'formae login --hosted' yet, so"
        echo "            /formae:setup will stop at 'unknown flag: --hosted'. That is the"
        echo "            expected outcome — set FORMAE_LOCAL_BIN to complete the journey."
    fi
fi
echo "Inside the session, run:"
if [ "$CLAUDE_AUTH" = "none" ]; then
    echo "  /login               # nothing is mounted, so sign in first (paste-code flow works headless)"
fi
echo "  /plugin marketplace add /mkt"
echo "  /plugin install formae@formae-dev"
echo "  /reload-plugins      # the MCP does not connect until the plugin is loaded"
echo "  /mcp                 # formae should show as connected"
if [ "$HOSTED" = "1" ]; then
    echo "  /formae:setup        # the onboarding journey"
else
    echo "  /formae:formae-status"
fi
echo

# shellcheck disable=SC2086  # intentional word-splitting of $MOUNTS
DEV_ENV=""
[ "$DEV_BUILD" = "1" ] && DEV_ENV="-e FORMAE_MCP_DEV=1"

HOSTED_ENV=""
if [ "$HOSTED" = "1" ]; then
    HOSTED_ENV="-e HOSTED=1 -e FORMAE_CLOUD_URL=$CLOUD_URL -e FORMAE_CLOUD_ISSUER=$CLOUD_ISSUER"
    # Mounted at a location resolve_formae probes, so the plugin treats it as the
    # user's own install and never downloads over it.
    if [ -n "$FORMAE_LOCAL_BIN" ]; then
        [ -x "$FORMAE_LOCAL_BIN" ] || { echo "FORMAE_LOCAL_BIN is not executable: $FORMAE_LOCAL_BIN" >&2; exit 1; }
        MOUNTS="$MOUNTS -v $FORMAE_LOCAL_BIN:/usr/local/bin/formae:ro"
        HOSTED_ENV="$HOSTED_ENV -e MOUNTED_FORMAE=1"
    fi
fi

# shellcheck disable=SC2086  # intentional word-splitting of $MOUNTS and $DEV_ENV
exec docker run -it --rm --network host \
  -e FORMAE_MCP_CHANNEL="$CHANNEL" \
  -e DEV_BUILD="$DEV_BUILD" \
  -e GO_VERSION="$GO_VERSION" \
  $DEV_ENV \
  $HOSTED_ENV \
  $MOUNTS \
  -v "$MKT:/mkt:ro" \
  "$IMAGE" bash -lc '
    set -e
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq curl ca-certificates git openssh-client >/dev/null 2>&1
    if [ "$DEV_BUILD" = "1" ]; then
      # The distro Go is far older than go.mod requires, so take the toolchain
      # from upstream. FORMAE_MCP_DEV builds the MCP from the plugin checkout.
      echo "installing go $GO_VERSION..."
      curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz" \
        | tar -C /usr/local -xz
      export PATH="/usr/local/go/bin:$PATH"
      echo "export PATH=/usr/local/go/bin:\$PATH" >> ~/.bashrc
      go version
    fi
    # Claude Code clones github plugin sources over SSH (git@github.com:...). A
    # fresh container has no known_hosts, so strict host-key checking aborts.
    # This repo is public, so force github clones over HTTPS (no auth, no SSH),
    # and seed known_hosts as a fallback for any SSH path that remains.
    git config --global url."https://github.com/".insteadOf "git@github.com:"
    git config --global url."https://github.com/".insteadOf "ssh://git@github.com/"
    mkdir -p ~/.ssh && ssh-keyscan -t rsa,ecdsa,ed25519 github.com >> ~/.ssh/known_hosts 2>/dev/null || true
    echo "installing Claude Code..."
    npm i -g @anthropic-ai/claude-code >/dev/null 2>&1

    # Completing a MOUNTED formae, and only that. A provisioned one already has
    # both halves: resolve_formae installs oidc alongside formae, and the launcher
    # puts that bin directory on PATH. A mount bypasses provisioning entirely, so
    # nothing has laid either down for it.
    if [ -n "${MOUNTED_FORMAE:-}" ]; then
      echo "completing the mounted formae..."
      mkdir -p /root/.formae-ai
      bash -c "$(curl -fsSL https://hub.platform.engineering/get/setup.sh)" -- install \
        --install-path /root/.formae-ai/opt --channel "$FORMAE_MCP_CHANNEL" --yes oidc \
        >/dev/null 2>&1 || echo "  (oidc install failed — the sign-in path names the remedy)"

      # The configured plugin dir is searched before the one derived from the
      # binary, and its default does not depend on where the binary is, so linking
      # it once serves the mount.
      mkdir -p /root/.pel/formae
      ln -sfn /root/.formae-ai/opt/formae/plugins /root/.pel/formae/plugins 2>/dev/null || true
      ls /root/.formae-ai/opt/formae/plugins 2>/dev/null | sed "s/^/  plugin: /"

      # Linked rather than put on PATH: prepending the provisioned tree would
      # shadow the mounted formae with the downloaded one.
      [ -x /root/.formae-ai/opt/bin/pkl ] && ln -sf /root/.formae-ai/opt/bin/pkl /usr/local/bin/pkl
      echo "  pkl: $(command -v pkl || echo MISSING)"
    fi

    echo "ready — marketplace at /mkt, channel='"$CHANNEL"'"
    if [ -z "$(ls -A /root/.claude 2>/dev/null)" ]; then
      echo "run:  /login   then   /plugin marketplace add /mkt   then   /plugin install formae@formae-dev"
    else
      echo "run:  /plugin marketplace add /mkt   then   /plugin install formae@formae-dev"
    fi
    exec claude
  '
