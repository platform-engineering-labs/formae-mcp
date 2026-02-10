# formae-mcp

MCP server and AI coding skills for [formae](https://formae.io), a modern infrastructure-as-code tool. Provides 15 MCP tools for querying and managing cloud infrastructure, plus 12 skills that teach your AI coding assistant how to perform common infrastructure workflows through formae.

## Prerequisites

- Go 1.25+
- A running formae agent (`formae agent start`)

## Installation

### Claude Code (via Plugin Marketplace)

Register the marketplace:

```
/plugin marketplace add platform-engineering-labs/formae-marketplace
```

Install the plugin:

```
/plugin install formae-mcp@formae-marketplace
```

Restart Claude Code for the plugin to take effect. The MCP server binary is built automatically on first use.

Verify by asking Claude to run `/formae-status`.

### Claude Code (manual)

If you prefer not to use the marketplace:

1. Clone the repo:

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.claude/plugins/formae-mcp
   ```

2. Start Claude Code with the plugin directory:

   ```bash
   claude --plugin-dir ~/.claude/plugins/formae-mcp
   ```

The MCP server binary is built automatically on first use.

### Codex

See [.codex/INSTALL.md](.codex/INSTALL.md) for Codex-specific installation instructions.

### OpenCode

See [.opencode/INSTALL.md](.opencode/INSTALL.md) for OpenCode-specific installation instructions.

## Available Skills

| Skill | Description |
|-------|-------------|
| `formae-status` | Check running commands, deployment progress, recent operations, and failures |
| `formae-stacks` | View infrastructure stacks, organization, and resource counts |
| `formae-resources` | Query deployed resources by type, stack, label, or management status |
| `formae-targets` | List cloud targets, configured regions, and provider accounts |
| `formae-plugins` | List installed plugins, supported providers, and resource types |
| `formae-apply` | Deploy infrastructure by applying a forma file or reconciling a stack |
| `formae-patch` | Make targeted infrastructure changes without a full reconcile |
| `formae-destroy` | Tear down infrastructure resources, stacks, or environments |
| `formae-drift` | Check for out-of-band changes and decide whether to absorb or overwrite |
| `formae-discover` | Find unmanaged resources in cloud accounts |
| `formae-import` | Bring unmanaged/discovered resources under formae management |
| `formae-plugin-new` | Scaffold a new formae resource plugin |

## Available MCP Tools

### Read-Only

| Tool | Description |
|------|-------------|
| `list_resources` | Query resources with optional filters |
| `list_stacks` | Retrieve all stacks |
| `list_targets` | Query configured cloud targets |
| `get_command_status` | Get status of a specific command |
| `list_commands` | List commands with optional query and filters |
| `get_agent_stats` | Retrieve agent statistics |
| `check_health` | Health check for the formae agent |
| `list_plugins` | List active plugins |
| `list_drift` | List infrastructure drift by stack |
| `extract_resources` | Extract resources as PKL code |

### Mutation

| Tool | Description |
|------|-------------|
| `apply_forma` | Deploy or update infrastructure (reconcile or patch mode) |
| `destroy_forma` | Remove infrastructure by file or query |
| `cancel_commands` | Cancel running commands |
| `force_sync` | Trigger immediate resource synchronization |
| `force_discover` | Trigger immediate resource discovery |

## Configuration

By default, formae-mcp connects to the formae agent at `http://localhost:49684`. To override this:

**Environment variables** (highest precedence):

```bash
export FORMAE_AGENT_URL=http://my-agent-host
export FORMAE_AGENT_PORT=8080
```

**Config file** (`~/.config/formae/formae.conf.pkl`):

```pkl
amends "formae:/Config.pkl"

cli {
  api {
    url = "http://my-agent-host"
    port = 8080
  }
}
```

Precedence: environment variables > config file > defaults.

## License

FSL-1.1-MIT
