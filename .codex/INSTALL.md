# Installing formae-mcp for Codex

## Prerequisites

- Go 1.25+
- Git
- A running formae agent (`formae agent start`)

## Installation

1. Install the MCP server binary:

   ```bash
   go install github.com/platform-engineering-labs/formae-mcp/cmd/formae-mcp@latest
   ```

2. Clone the repo:

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.codex/formae-mcp
   ```

3. Symlink skills into Codex:

   ```bash
   mkdir -p ~/.agents/skills
   ln -s ~/.codex/formae-mcp/skills ~/.agents/skills/formae-mcp
   ```

4. Register the MCP server. The skills drive the `formae-mcp` tools, so Codex
   must know how to start the server. Either use the CLI:

   ```bash
   codex mcp add formae -- formae-mcp
   ```

   or merge this block into `~/.codex/config.toml` (add to the existing file —
   don't replace it):

   ```toml
   [mcp_servers.formae]
   command = "formae-mcp"
   ```

   `formae-mcp` must be resolvable on your `PATH` (`go install` puts it in
   `$(go env GOPATH)/bin`). If Codex can't find it — GUI/IDE launches often have
   a narrower `PATH` — use the absolute path instead of the bare name (run
   `go env GOPATH` to find it, then point at `<gopath>/bin/formae-mcp`).

5. Restart Codex.

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

```bash
cd ~/.codex/formae-mcp && git pull && go install ./cmd/formae-mcp/
```

## Uninstalling

```bash
codex mcp remove formae
rm ~/.agents/skills/formae-mcp
rm -rf ~/.codex/formae-mcp
```

If you registered the server by editing `~/.codex/config.toml`, delete the
`[mcp_servers.formae]` block instead of running `codex mcp remove`.

Optionally remove the binary:

```bash
rm "$(go env GOPATH)/bin/formae-mcp"
```
