# Installing the formae MCP for Codex

## Prerequisites

- A recent Codex CLI (verified with 0.148.0) that supports `codex plugin` and
  Claude-format plugin marketplaces.
- A running formae agent (`formae agent start`) and a formae profile pointing at it.

## Installation

```bash
codex plugin marketplace add platform-engineering-labs/formae-marketplace
codex plugin add formae@formae-marketplace
```

That's it. The skills and the MCP server both come with the plugin — no
cloning, no symlinking, no editing `~/.codex/config.toml`, and nothing needs to
be on your `PATH`.

The first session after install downloads the prebuilt `formae-mcp` binary
(and a matched `formae`) into `~/.formae-ai/opt`. The plugin manifest declares
the server as `required` with a 120-second `startup_timeout_sec`, so Codex
waits for that one-time download instead of starting the session without
formae tools. Later sessions start instantly.

## Verify

Two checks — the first confirms Codex installed the plugin, the second
confirms it actually works end-to-end.

1. Confirm the plugin is installed:

   ```bash
   codex plugin list
   ```

   `formae` should appear, sourced from `formae-marketplace`.

2. With a formae agent running (`formae agent start`), start a Codex session
   and ask for formae status (e.g. "what formae commands are running?") or a
   hub search (e.g. "search the formae hub for aws plugins"), and confirm a
   real tool call returns data — not just that the skill loaded.

## Updating

```bash
codex plugin marketplace upgrade
codex plugin remove formae@formae-marketplace
codex plugin add formae@formae-marketplace
```

## Uninstalling

```bash
codex plugin remove formae@formae-marketplace
codex plugin marketplace remove formae-marketplace
```

Optionally remove the downloaded binaries:

```bash
rm -rf ~/.formae-ai/opt
```

## Manual install (older Codex versions)

If your Codex CLI predates plugin marketplace support, register the MCP
server by hand instead.

1. Clone the repo (it carries the skills and the launcher):

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.codex/formae
   ```

2. Symlink the skills into Codex:

   ```bash
   mkdir -p ~/.agents/skills
   ln -s ~/.codex/formae/skills ~/.agents/skills/formae
   ```

3. Register the MCP server. Point Codex at the launcher script, which downloads
   the prebuilt `formae-mcp` (and a matched `formae`) into `~/.formae-ai/opt` on
   first run and starts the server. Merge this block into `~/.codex/config.toml`
   (add to the existing file — don't replace it):

   ```toml
   [mcp_servers.formae]
   command = "/home/you/.codex/formae/scripts/start-mcp.sh"
   required = true            # wait for the server at session start
   startup_timeout_sec = 120  # headroom for the one-time binary download
   ```

   Use an **absolute** path to `start-mcp.sh` (expand `~`), and make sure it is
   executable (`chmod +x`). No binary needs to be on your `PATH`.

   Both extra settings matter: Codex starts sessions without waiting for MCP
   servers, so without `required = true` the first session (while the binaries
   download) silently has no formae tools. `startup_timeout_sec` gives that
   one-time download room; later launches start in milliseconds. If you prefer
   registering via the CLI, run
   `codex mcp add formae -- ~/.codex/formae/scripts/start-mcp.sh`, then add the
   `required` and `startup_timeout_sec` lines to the generated
   `[mcp_servers.formae]` block by hand (the CLI cannot set them).

4. Verify with `codex mcp list`, then confirm end-to-end with a live tool call
   as described above.

5. To update, pull the latest skills and launcher:

   ```bash
   cd ~/.codex/formae && git pull
   ```

   On the next launch, if the plugin version changed, the launcher automatically
   downloads the matching `formae-mcp` binary. To update the `formae` binary when
   the connected agent is newer, run the `/formae:upgrade` skill (it asks first).

6. To uninstall:

   ```bash
   codex mcp remove formae
   rm ~/.agents/skills/formae
   rm -rf ~/.codex/formae
   ```

   If you registered the server by editing `~/.codex/config.toml`, delete the
   `[mcp_servers.formae]` block instead of running `codex mcp remove`.

   Optionally remove the downloaded binaries:

   ```bash
   rm -rf ~/.formae-ai/opt
   ```
