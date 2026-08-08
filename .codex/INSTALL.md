# Installing the formae MCP for Codex

## Prerequisites

- Git
- `curl`
- A running formae agent (`formae agent start`) and a formae profile pointing at it

You do **not** need a Go toolchain. The MCP server and a matched `formae` binary
are downloaded as prebuilt artifacts on first launch — nothing is compiled on your
machine.

## Installation

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
   first run and starts the server. Either use the CLI:

   ```bash
   codex mcp add formae -- ~/.codex/formae/scripts/start-mcp.sh
   ```

   or merge this block into `~/.codex/config.toml` (add to the existing file —
   don't replace it):

   ```toml
   [mcp_servers.formae]
   command = "/home/you/.codex/formae/scripts/start-mcp.sh"
   ```

   Use an **absolute** path to `start-mcp.sh` (expand `~`), and make sure it is
   executable (`chmod +x`). No binary needs to be on your `PATH`.

## Verify

Two checks — the first confirms Codex sees the server, the second confirms it
actually works end-to-end.

1. Confirm the server is registered:

   ```bash
   codex mcp list
   ```

   `formae` should appear in the list.

2. With a formae agent running (`formae agent start`), ask Codex for formae
   status (invoke the `formae-status` skill, e.g. "what formae commands are
   running?") and confirm a tool call returns live agent data — not just that
   the skill loaded.

## Updating

Pull the latest skills and launcher:

```bash
cd ~/.codex/formae && git pull
```

On the next launch, if the plugin version changed, the launcher automatically
downloads the matching `formae-mcp` binary. To update the `formae` binary when the
connected agent is newer, run the `/formae:upgrade` skill (it asks first).

## Uninstalling

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
