# formae-mcp

MCP server and AI coding skills for the Infrastructure-as-code (IaC) platform [formae](https://formae.io). Provides 31 MCP tools for querying and managing cloud infrastructure, plus 19 skills that teach your AI coding assistant how to perform common infrastructure workflows through formae.

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

Run `/reload-plugins` (Claude Code v2.1.116+) to apply the install without restarting your session. On older versions, restart Claude Code instead. The MCP server binary is built automatically on first use.

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

### Authoring

`formae-author` is the front door for authoring new infrastructure with formae. Tell it what you want to deploy ("I want to deploy a static website with CloudFront") and it triages the work: infers which plugin schema dependencies are needed, and dispatches to focused skills — `formae-project-init` to scaffold a new forma project, `formae-deps` to resolve plugin and PKL schema dependencies, `formae-stack-design` to design and write the forma file, `formae-policy` to attach lifecycle policies, and `formae-plugin-new` when a required resource type has no existing plugin. For existing cloud resources, it hands off to `formae-import` to bring them under management. The authoring skills are backed by hub tools (`search_hub_plugins`, `list_plugin_examples`) that pull the live plugin catalog and version-matched examples directly from the formae hub.

### All Skills

| Skill | Description |
|-------|-------------|
| `formae-author` | Front door for authoring new infrastructure: triages intent, infers deps, dispatches to focused skills |
| `formae-project-init` | Scaffold a new forma project with the correct directory layout and config |
| `formae-deps` | Resolve and install plugin and PKL schema dependencies for a forma project |
| `formae-stack-design` | Design and write a forma file for a given set of infrastructure requirements |
| `formae-status` | Check running commands, deployment progress, recent operations, and failures |
| `formae-stacks` | View infrastructure stacks, organization, and resource counts |
| `formae-resources` | Query deployed resources by type, stack, label, or management status |
| `formae-targets` | List cloud targets, configured regions, and provider accounts |
| `formae-apply` | Deploy infrastructure by applying a forma file or reconciling a stack |
| `formae-patch` | Make targeted infrastructure changes without a full reconcile |
| `formae-rename` | Rename a resource's label via `alias` without destroying the cloud object |
| `formae-destroy` | Tear down infrastructure resources, stacks, or environments |
| `formae-fix-code-drift` | Check for out-of-band changes and decide whether to absorb or overwrite |
| `formae-policy` | Set, remove, or inspect TTL and auto-reconcile policies — inline on one stack, or standalone and reused across several |
| `formae-discover` | Find unmanaged resources in cloud accounts |
| `formae-import` | Bring unmanaged/discovered resources under formae management |
| `formae-plugin-new` | Scaffold a new formae resource plugin |
| `formae-plugin-add-resource` | Add a new resource type to an existing plugin |
| `formae-config` | Switch, list, save, create, delete, compare, view, and edit named formae configuration profiles (drives `formae profile`; requires formae >= 0.87.0) |

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
| `list_changes_since_last_reconcile` | List infrastructure changes since last reconcile |
| `extract_resources` | Extract resources as PKL code |
| `list_policies` | List standalone (reusable) policies and the stacks they're attached to |
| `search_hub_plugins` | Search the live formae hub plugin catalog by keyword or resource type |
| `get_hub_plugin` | Get details for a specific plugin from the hub |
| `list_plugin_examples` | List version-matched examples for a hub plugin |
| `get_plugin_example` | Fetch a specific example from the hub |

### Mutation

| Tool | Description |
|------|-------------|
| `apply_forma` | Deploy or update infrastructure (reconcile or patch mode) |
| `destroy_forma` | Remove infrastructure by file or query |
| `cancel_commands` | Cancel running commands |
| `force_sync` | Trigger immediate resource synchronization |
| `force_discover` | Trigger immediate resource discovery |
| `force_check_ttl` | Trigger an immediate TTL expiry sweep across all stacks |
| `force_reconcile_stack` | Force a one-shot reconcile on a stack (requires auto-reconcile policy attached) |
| `create_inline_policy` | Plan a TTL or auto-reconcile policy edit on a stack (returns snippet + insertion anchor; caller applies via Edit) |
| `create_standalone_policy` | Plan the declaration of a reusable policy in a forma file (returns snippet + insertion anchor) |
| `attach_standalone_policy` | Plan the attachment of a standalone policy to a stack |
| `detach_standalone_policy` | Plan the detachment of a standalone policy from a stack |
| `delete_standalone_policy` | Plan the deletion of an unattached standalone policy (returns source anchor + a destroy forma) |

### Profiles (requires formae >= 0.87.0)

Manage named formae environments (endpoint + targets) from your assistant.

| Tool | Description |
|------|-------------|
| `list_profiles` | List configuration profiles and which one is active |
| `current_profile` | Show the active profile |
| `use_profile` | Switch the active profile (global; only on explicit "change my default" requests) |
| `save_profile` | Snapshot the active profile under a new name |
| `create_profile` | Create a new profile from the starter template |
| `delete_profile` | Delete a profile (cannot be the active one) |
| `diff_profiles` | Compare two profiles (or one against the active) |
| `read_profile` | Return a profile's PKL contents |
| `write_profile` | Replace a profile's PKL (overwrite-only; refuses the active profile) |

## Configuration

By default, formae-mcp connects to the formae agent at `http://localhost:49684`. To override this:

**Environment variables** (highest precedence):

```bash
export FORMAE_AGENT_URL=http://my-agent-host
export FORMAE_AGENT_PORT=8080
```

**Profile** (formae >= 0.87.0): when no environment variables are set, formae-mcp reads the agent endpoint from your **active** formae profile — or from the profile named by a tool's `profile` argument. Profiles live at `~/.config/formae/profiles/<name>.pkl` and are managed with `formae profile` (or the profile tools above); each looks like:

```pkl
amends "formae:/Config.pkl"

cli {
  api {
    url = "http://my-agent-host"
    port = 8080
  }
}
```

Precedence: environment variables > per-call `profile` / active profile > `http://localhost:49684` default.

## License

[FSL-1.1-ALv2](LICENSE)
