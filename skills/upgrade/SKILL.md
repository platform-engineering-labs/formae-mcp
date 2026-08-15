---
name: upgrade
description: Use to upgrade the local formae when the connected agent is newer (classic mode) — always confirms before touching anything and warns that it may move a pinned formae.
---

# Upgrade formae (channel-aware, non-silent)

Triggered when `check_health` or a version-skew notice reports that the connected agent is newer than the local `formae` binary.

## Step 1 — Show the situation and get confirmation

Before doing anything:

1. Run the `check_health` MCP tool to retrieve the current local `formae` version and the connected agent version.
2. Present both versions clearly, for example:
   ```
   Local formae : 0.84.1
   Agent version: 0.87.0
   ```
3. State the plan: "I will upgrade your local formae to match the agent."
4. Warn the user: upgrading may move a `formae` they deliberately pinned to an older version in order to stay compatible with a specific agent. If they are running a self-managed agent they want to keep in sync at a specific version, they should upgrade the agent first or decline here.
5. **Ask for explicit confirmation.** Do NOT proceed without it. Never upgrade silently in classic mode.

## Step 2 — Determine where formae lives

**Do not probe the filesystem for this.** There is exactly one formae per machine and the launcher already decided which one; a machine can have a leftover copy in the managed tree that is *not* the binary in use, so `test -x ~/.formae-ai/opt/bin/formae` answers the wrong question and would send you at a binary the MCP never runs.

Read the answer out of the `check_health` output from Step 1. There are two shapes it can take, because a formae below the MCP's floor fails before any skew notice can be produced:

- **`check_health` errored with "formae is too old"** — the message names the binary and whose it is. That is the strongest signal: the MCP cannot work at all until it is upgraded. Follow the route the message gives.
- **`check_health` succeeded with a skew notice** — the agent is merely newer than formae. If the notice says "Run /formae:upgrade to update formae" it is the managed install (Step 3a); if it names a path and says the plugin will not change it, it is the user's own (Step 3b, using that path).

If `check_health` succeeded and there is no skew notice, there is nothing to upgrade — say so and stop.

For the user's own install, do **not** assume sudo. Ask where it came from, or look at the path: a binary under `$HOME` is theirs to replace without privilege, while one under `/opt` or `/usr/local` will need it.

## Step 3a — Upgrade a managed-tree install (sudo-free)

Source the bundled helper and call `provision_pkg` with `FORMAE_FORCE_PROVISION=1` so it reinstalls even when the binary is already present:

```sh
sh -c '. "$CLAUDE_PLUGIN_ROOT/scripts/provision.sh"; FORMAE_FORCE_PROVISION=1 provision_pkg formae "${FORMAE_MCP_CHANNEL:-stable}"'
```

If the MCP binary itself is also behind the agent, upgrade it the same way:

```sh
sh -c '. "$CLAUDE_PLUGIN_ROOT/scripts/provision.sh"; FORMAE_FORCE_PROVISION=1 provision_pkg formae-mcp "${FORMAE_MCP_CHANNEL:-stable}"'
```

Notes:
- `provision_pkg <pkg> <channel>` installs `<pkg>` from `<channel>` into `~/.formae-ai/opt`; the binary lands at `~/.formae-ai/opt/bin/<pkg>`.
- `FORMAE_FORCE_PROVISION=1` bypasses the fast-path skip that would leave an already-present binary untouched.
- `$CLAUDE_PLUGIN_ROOT` is set by the plugin runtime to the plugin's root directory; `scripts/provision.sh` lives there.
- The channel defaults to `stable`; it can be overridden with `FORMAE_MCP_CHANNEL` (e.g. `dev`).
- Both commands are sudo-free and safe to run in any terminal or tool-call.

## Step 3b — User-managed install (outside the managed tree)

Do NOT attempt a non-interactive `sudo` command. Instead, tell the user to run the upgrade themselves in a terminal. Provide the appropriate command for their install:

- If they used the hub one-liner originally:
  ```sh
  bash -c "$(curl -fsSL https://hub.platform.engineering/get/setup.sh)" -- install --channel stable --yes formae
  ```
- If they have a `formae update` subcommand available:
  ```sh
  sudo formae update --channel stable
  ```

Repeat the pinned-formae warning: if they pinned this version intentionally, upgrading here will replace it.

Once they confirm the upgrade is done, move to Step 4.

## Step 4 — Confirm the skew is resolved

After the upgrade (either path):

1. Re-run `check_health` to confirm the version-skew notice is gone.
2. If the skew notice is still present for `formae`, report the updated local version. The notice names the binary actually in use, so compare that against what you upgraded.
3. If the MCP binary (`formae-mcp`) was also upgraded, note that the new MCP binary only takes effect on the **next launch** of the harness — the running instance cannot hot-swap itself. The local `formae` version is picked up immediately on the next tool call (the version cache is keyed on path + mtime + size, so the new binary is seen without restarting).
4. Report the final state clearly.
