# Installing formae-mcp for OpenCode

## Prerequisites

- Go 1.25+
- Git
- OpenCode
- A running formae agent (`formae agent start`)

## Installation

1. Install the MCP server binary:

   ```bash
   go install github.com/platform-engineering-labs/formae-mcp/cmd/formae-mcp@latest
   ```

2. Clone the repo:

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.config/opencode/formae-mcp
   ```

3. Symlink skills into OpenCode:

   ```bash
   mkdir -p ~/.config/opencode/skills
   ln -s ~/.config/opencode/formae-mcp/skills ~/.config/opencode/skills/formae-mcp
   ```

4. Register the MCP server. The skills drive the `formae-mcp` tools, so OpenCode
   must know how to start the server. Merge this into
   `~/.config/opencode/opencode.json` (add to your existing config — don't
   overwrite other keys):

   ```json
   {
     "$schema": "https://opencode.ai/config.json",
     "mcp": {
       "formae": { "type": "local", "command": ["formae-mcp"], "enabled": true }
     }
   }
   ```

   `formae-mcp` must be resolvable on your `PATH` (`go install` puts it in
   `$(go env GOPATH)/bin`). If OpenCode can't find it — GUI/IDE launches often
   have a narrower `PATH` — use the absolute path as the command element instead
   of the bare name (run `go env GOPATH`, then use `["<gopath>/bin/formae-mcp"]`).

5. Restart OpenCode.

## Verify

Two checks — the first confirms OpenCode sees the server, the second confirms it
actually works end-to-end.

1. Confirm the server is registered and connected:

   ```bash
   opencode mcp list
   ```

   `formae` should appear in the list.

2. With a formae agent running (`formae agent start`), ask OpenCode for formae
   status (invoke the `formae-status` skill, e.g. "what formae commands are
   running?") and confirm a tool call returns live agent data — not just that
   the skill loaded.

## Updating

```bash
cd ~/.config/opencode/formae-mcp && git pull && go install ./cmd/formae-mcp/
```

## Uninstalling

Delete the `mcp.formae` block from `~/.config/opencode/opencode.json`, then
remove the skills symlink and clone:

```bash
rm ~/.config/opencode/skills/formae-mcp
rm -rf ~/.config/opencode/formae-mcp
```

Optionally remove the binary:

```bash
rm "$(go env GOPATH)/bin/formae-mcp"
```
