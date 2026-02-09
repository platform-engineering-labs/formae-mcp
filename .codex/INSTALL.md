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

4. Restart Codex.

## Verify

```bash
ls -la ~/.agents/skills/formae-mcp
```

You should see the skill directories (formae-status, formae-apply, etc.).

## Updating

```bash
cd ~/.codex/formae-mcp && git pull && go install ./cmd/formae-mcp/
```

## Uninstalling

```bash
rm ~/.agents/skills/formae-mcp
rm -rf ~/.codex/formae-mcp
```

Optionally remove the binary:

```bash
rm "$(go env GOPATH)/bin/formae-mcp"
```
