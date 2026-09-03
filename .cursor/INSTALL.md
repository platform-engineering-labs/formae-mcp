# Installing formae-mcp for Cursor

## Prerequisites

- Git
- `curl`
- Cursor 2.4 or later, which is where Agent Skills arrived
- A running formae agent (`formae agent start`) and a formae profile pointing at it

You do **not** need a Go toolchain, and you do not install formae separately. The
MCP server, a matched `formae` binary, and the `oidc` auth plugin that a hosted
sign-in needs are all downloaded as prebuilt artifacts on first launch — nothing
is compiled on your machine.

## Installation

1. Clone the repo (it carries the skills and the launcher):

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.cursor/formae-mcp
   ```

2. Symlink skills into Cursor:

   ```bash
   mkdir -p ~/.agents/skills
   ln -s ~/.cursor/formae-mcp/skills ~/.agents/skills/formae-mcp
   ```

   `~/.agents/skills` is the cross-agent location, so one symlink serves Cursor
   and Codex together. Cursor walks the root recursively, so the nesting this
   creates (`formae-mcp/<skill>/SKILL.md`) is picked up as normal.

3. Register the MCP server. Point Cursor at the launcher script, which downloads
   the prebuilt `formae-mcp` (plus a matched `formae` and the `oidc` plugin) into
   `~/.formae-ai/opt` on first run and starts the server. Merge this into
   `~/.cursor/mcp.json` for every project, or `.cursor/mcp.json` in one project's
   root to scope it there — add to the existing file, don't replace it:

   ```json
   {
     "mcpServers": {
       "formae": {
         "command": "/home/you/.cursor/formae-mcp/scripts/start-mcp.sh"
       }
     }
   }
   ```

   Use an **absolute** path (expand `~`), and make sure the script is executable
   (`chmod +x`). Cursor has no CLI for this; the file (or Settings) is the way in.

   **The launcher is what makes a desktop-launched Cursor work.** Cursor starts
   from a desktop session rather than your shell, so its `PATH` is narrower than
   the one you see in a terminal. Nothing here needs to be on it: the launcher
   resolves its own binaries by absolute path and puts formae's own `bin`
   directory on `PATH` for the server it starts. That last part matters more than
   it looks — formae shells out to `pkl` by bare name to read a plugin's
   manifest, which is how an auth plugin is recognised as one, so a server
   started without it reports the `oidc` plugin as missing while it sits right
   there on disk.

4. Restart Cursor.

## Verify

Two checks — the first confirms Cursor sees the server, the second confirms it
actually works end-to-end.

1. Open Settings and confirm `formae` is listed under MCP with its tools
   enumerated. A server that failed to start appears with an error rather than
   a tool list, which is the distinction worth looking for.

2. With a formae agent running (`formae agent start`), ask Cursor for formae
   status ("what formae commands are running?") and confirm a tool call returns
   live agent data — not just that the skill loaded.

## Known limitation: skills that name slash commands

Several skills refer to each other as `/formae:<name>`, which is Claude Code's
invocation syntax and means nothing in Cursor. The skills themselves load and
work; a cross-reference to another one is what does not, so following a chain
between skills may need you to name the next one yourself. Ask for it by
description ("import these resources into my codebase") rather than by slash
command.

## Updating

Pull the latest skills and launcher:

```bash
cd ~/.cursor/formae-mcp && git pull
```

On the next launch, if the plugin version changed, the launcher automatically
downloads the matching `formae-mcp` binary. To update the `formae` binary when
the connected agent is newer, run the `formae:upgrade` skill (it asks first).

## Uninstalling

```bash
rm ~/.agents/skills/formae-mcp
rm -rf ~/.cursor/formae-mcp
```

Then delete the `formae` entry from `~/.cursor/mcp.json` (or the project's
`.cursor/mcp.json`).

Optionally remove the downloaded binaries:

```bash
rm -rf ~/.formae-ai/opt
```
