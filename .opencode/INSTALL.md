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

4. Restart OpenCode.

## Verify

```bash
ls -la ~/.config/opencode/skills/formae-mcp
```

You should see the skill directories (formae-status, formae-apply, etc.).

## Updating

```bash
cd ~/.config/opencode/formae-mcp && git pull && go install ./cmd/formae-mcp/
```

## Uninstalling

```bash
rm ~/.config/opencode/skills/formae-mcp
rm -rf ~/.config/opencode/formae-mcp
```

Optionally remove the binary:

```bash
rm "$(go env GOPATH)/bin/formae-mcp"
```
