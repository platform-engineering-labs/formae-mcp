# Installing formae-mcp for Cursor

## Prerequisites

- Go 1.25+
- Git
- A running formae agent (`formae agent start`)
- Cursor 2.4 or later, which is where Agent Skills arrived

## Installation

1. Install the MCP server binary:

   ```bash
   go install github.com/platform-engineering-labs/formae-mcp/cmd/formae-mcp@latest
   ```

2. Clone the repo:

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.cursor/formae-mcp
   ```

3. Symlink skills into Cursor:

   ```bash
   mkdir -p ~/.agents/skills
   ln -s ~/.cursor/formae-mcp/skills ~/.agents/skills/formae-mcp
   ```

   `~/.agents/skills` is the cross-agent location, so one symlink serves Cursor
   and Codex together. Cursor walks the root recursively, so the nesting this
   creates (`formae-mcp/<skill>/SKILL.md`) is picked up as normal.

4. Register the MCP server. The skills drive the `formae-mcp` tools, so Cursor
   must know how to start the server. Merge this into `~/.cursor/mcp.json` for
   every project, or `.cursor/mcp.json` in one project's root to scope it there
   — add to the existing file, don't replace it:

   ```json
   {
     "mcpServers": {
       "formae": {
         "command": "formae-mcp"
       }
     }
   }
   ```

   Cursor has no CLI for this; the file (or Settings) is the way in.

   `formae-mcp` must be resolvable on your `PATH` (`go install` puts it in
   `$(go env GOPATH)/bin`). Cursor is launched from a desktop session rather
   than your shell, so its `PATH` is often narrower than the one you see in a
   terminal — if the server fails to start, use the absolute path instead of
   the bare name (run `go env GOPATH`, then point at `<gopath>/bin/formae-mcp`).

5. Restart Cursor.

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

```bash
cd ~/.cursor/formae-mcp && git pull && go install ./cmd/formae-mcp/
```

## Uninstalling

```bash
rm ~/.agents/skills/formae-mcp
rm -rf ~/.cursor/formae-mcp
```

Then delete the `formae` entry from `~/.cursor/mcp.json` (or the project's
`.cursor/mcp.json`).

Optionally remove the binary:

```bash
rm "$(go env GOPATH)/bin/formae-mcp"
```
