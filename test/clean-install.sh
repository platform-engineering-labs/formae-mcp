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
#   ISOLATE_CLAUDE=1  mount a throwaway copy of ~/.claude instead of the real
#                     one, so installing a plugin in the container cannot
#                     disturb the host's own plugin set. Auth still works: the
#                     copy carries the same credentials.
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
# Requires: docker, a logged-in Claude Code on the host (~/.claude), and the
# chosen ref pushed to origin (the repo must be reachable — public, or auth'd).
set -eu

CHANNEL="${1:-dev}"
REF="${2:-$(git rev-parse --abbrev-ref HEAD)}"
REPO="${REPO:-platform-engineering-labs/formae-mcp}"
IMAGE="${IMAGE:-node:22-bookworm}"
DEV_BUILD="${DEV_BUILD:-0}"
MOUNT_PROFILES="${MOUNT_PROFILES:-1}"
ISOLATE_CLAUDE="${ISOLATE_CLAUDE:-0}"
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
CLAUDE_HOME="$HOME"
if [ "$ISOLATE_CLAUDE" = "1" ]; then
    # A copy, so a plugin installed in here cannot land in the host's config.
    CLAUDE_HOME="$(mktemp -d)"
    [ -e "$HOME/.claude" ]      && cp -a "$HOME/.claude" "$CLAUDE_HOME/.claude"
    [ -e "$HOME/.claude.json" ] && cp -a "$HOME/.claude.json" "$CLAUDE_HOME/.claude.json"
    echo "Isolated Claude config: $CLAUDE_HOME (throwaway copy)"
fi
[ -e "$CLAUDE_HOME/.claude" ]      && MOUNTS="$MOUNTS -v $CLAUDE_HOME/.claude:/root/.claude"
[ -e "$CLAUDE_HOME/.claude.json" ] && MOUNTS="$MOUNTS -v $CLAUDE_HOME/.claude.json:/root/.claude.json"
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
if [ "$ISOLATE_CLAUDE" = "1" ]; then
    echo "  claude config: throwaway copy — host plugin set untouched"
else
    echo "  claude config: host ~/.claude mounted read-write — installs land on the host too"
fi
if [ "$HOSTED" = "1" ]; then
    echo "  hosted:   control plane $CLOUD_URL (issuer $CLOUD_ISSUER), oidc plugin installed"
    if [ -n "$FORMAE_LOCAL_BIN" ]; then
        echo "  formae:   mounted from $FORMAE_LOCAL_BIN — NOT downloaded (a local binary is 'your own install')"
    else
        echo "  formae:   pulled from the '$CHANNEL' channel."
        echo "            NOTE: released binaries have no 'formae login --hosted' yet, so"
        echo "            /formae:setup will stop at 'unknown flag: --hosted'. That is the"
        echo "            expected outcome — set FORMAE_LOCAL_BIN to complete the journey."
    fi
fi
echo "Inside the session, run:"
echo "  /plugin marketplace add /mkt"
echo "  /plugin install formae@formae-dev"
echo "  /reload-plugins ; /mcp"
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

    if [ "${HOSTED:-0}" = "1" ]; then
      # The oidc auth plugin is what a hosted sign-in drives, and nothing else
      # here installs it: formae'"'"'s own package requires only pkl, so a freshly
      # provisioned tree has no plugins at all. Installed up front so the journey
      # is not interrupted by a privilege prompt half-way through a sign-in — the
      # same reason it belongs in the standard bundle.
      #
      # Into the same user tree the plugin provisions into, so it needs no sudo
      # and is found by the discovery that derives from the binary'"'"'s location.
      echo "installing the oidc auth plugin..."
      mkdir -p /root/.formae-ai
      bash -c "$(curl -fsSL https://hub.platform.engineering/get/setup.sh)" -- install         --install-path /root/.formae-ai/opt --channel "$FORMAE_MCP_CHANNEL" --yes oidc         >/dev/null 2>&1 || echo "  (oidc install failed — sign-in will report the remedy)"
      # Discovery searches the configured plugin dir before the one derived from
      # the binary'"'"'s own location, and the configured default is the same in both
      # modes. Linking it once therefore covers a mounted formae and a downloaded
      # one, where linking the binary-derived path would only cover whichever we
      # happened to run.
      mkdir -p /root/.pel/formae
      ln -sfn /root/.formae-ai/opt/formae/plugins /root/.pel/formae/plugins 2>/dev/null || true
      ls /root/.formae-ai/opt/formae/plugins 2>/dev/null | sed "s/^/  plugin: /"

      # pkl has to be on PATH, and this is not incidental: classifying a plugin as
      # an auth plugin means evaluating its formae-plugin.pkl manifest, which
      # shells out to `pkl` found on PATH. Without it every auth plugin is
      # invisible and a sign-in reports the plugin as not installed — pointing at
      # an install command that cannot help, because it already is.
      #
      # Linked rather than exported, so it survives into whatever shell or child
      # the MCP launches, and so it cannot shadow a formae mounted at
      # /usr/local/bin by putting the provisioned tree ahead of it on PATH.
      [ -x /root/.formae-ai/opt/bin/pkl ] && ln -sf /root/.formae-ai/opt/bin/pkl /usr/local/bin/pkl
      echo "  pkl: $(command -v pkl || echo MISSING)"
    fi

    echo "ready — marketplace at /mkt, channel='"$CHANNEL"'"
    echo "run:  /plugin marketplace add /mkt   then   /plugin install formae@formae-dev"
    exec claude
  '
