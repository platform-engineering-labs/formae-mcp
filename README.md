# formae-mcp

MCP server and AI coding skills for the Infrastructure-as-code (IaC) platform [formae](https://formae.io). Provides 31 MCP tools for querying and managing cloud infrastructure, plus 19 skills that teach your AI coding assistant how to perform common infrastructure workflows through formae.

## Prerequisites

- A running formae agent (`formae agent start`) and a formae profile pointing at it

On first use the plugin downloads a prebuilt MCP server into `~/.formae-ai/opt` —
no Go toolchain required. If you have no `formae` installed it downloads one there
too; if you already have one, that is the one it uses and nothing is downloaded.
(A Go toolchain is only needed for local development builds, via `FORMAE_MCP_DEV=1`.)

## Installation

### Claude Code (via Plugin Marketplace)

Register the marketplace:

```
/plugin marketplace add platform-engineering-labs/formae-marketplace
```

Install the plugin:

```
/plugin install formae@formae-marketplace
```

Run `/reload-plugins` (Claude Code v2.1.116+) to apply the install without restarting your session. On older versions, restart Claude Code instead. On first use the plugin downloads a prebuilt MCP server into `~/.formae-ai/opt`, plus a `formae` if you do not already have one — nothing is compiled on your machine.

Verify by asking Claude to run `/formae:formae-status`.

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

On first use the plugin downloads a prebuilt MCP server into `~/.formae-ai/opt`, plus a `formae` if you do not already have one; set `FORMAE_MCP_DEV=1` to build from source instead.

### Codex

See [.codex/INSTALL.md](.codex/INSTALL.md) for Codex-specific installation instructions.

### OpenCode

See [.opencode/INSTALL.md](.opencode/INSTALL.md) for OpenCode-specific installation instructions.

### Cursor

See [.cursor/INSTALL.md](.cursor/INSTALL.md) for Cursor-specific installation instructions.

## Migration from `formae-mcp`

The plugin was previously distributed under the name `formae-mcp`. If you installed it under that name, re-add it under the new name `formae`:

```
/plugin install formae@formae-marketplace
```

Then remove the old entry:

```
/plugin remove formae-mcp
```

Run `/reload-plugins` to apply the change without restarting your session.

## Available Skills

### Authoring

`/formae:formae-author` is the front door for authoring new infrastructure with formae. Tell it what you want to deploy ("I want to deploy a static website with CloudFront") and it triages the work: infers which plugin schema dependencies are needed, and dispatches to focused skills — `/formae:formae-project-init` to scaffold a new forma project, `/formae:formae-deps` to resolve plugin and PKL schema dependencies, `/formae:formae-stack-design` to design and write the forma file, `/formae:formae-policy` to attach lifecycle policies, and `/formae:formae-plugin-new` when a required resource type has no existing plugin. For existing cloud resources, it hands off to `/formae:formae-import` to bring them under management. The authoring skills are backed by hub tools (`search_hub_plugins`, `list_plugin_examples`) that pull the live plugin catalog and version-matched examples directly from the formae hub.

### All Skills

| Skill | Description |
|-------|-------------|
| `/formae:formae-author` | Front door for authoring new infrastructure: triages intent, infers deps, dispatches to focused skills |
| `/formae:formae-project-init` | Scaffold a new forma project with the correct directory layout and config |
| `/formae:formae-deps` | Resolve and install plugin and PKL schema dependencies for a forma project |
| `/formae:formae-stack-design` | Design and write a forma file for a given set of infrastructure requirements |
| `/formae:formae-status` | Check running commands, deployment progress, recent operations, and failures |
| `/formae:formae-stacks` | View infrastructure stacks, organization, and resource counts |
| `/formae:formae-resources` | Query deployed resources by type, stack, label, or management status |
| `/formae:formae-targets` | List cloud targets, configured regions, and provider accounts |
| `/formae:formae-apply` | Deploy infrastructure by applying a forma file or reconciling a stack |
| `/formae:formae-patch` | Make targeted infrastructure changes without a full reconcile |
| `/formae:formae-rename` | Rename a resource's label via `alias` without destroying the cloud object |
| `/formae:formae-destroy` | Tear down infrastructure resources, stacks, or environments |
| `/formae:formae-fix-code-drift` | Check for out-of-band changes and decide whether to absorb or overwrite |
| `/formae:formae-policy` | Set, remove, or inspect TTL and auto-reconcile policies — inline on one stack, or standalone and reused across several |
| `/formae:formae-discover` | Find unmanaged resources in cloud accounts |
| `/formae:formae-import` | Bring unmanaged/discovered resources under formae management |
| `/formae:formae-plugin-new` | Scaffold a new formae resource plugin |
| `/formae:formae-plugin-add-resource` | Add a new resource type to an existing plugin |
| `/formae:formae-config` | Switch, list, save, create, delete, compare, view, and edit named formae configuration profiles (drives `formae profile`; requires formae >= 0.87.0) |
| `/formae:setup` | Install or repair the formae MCP — ensures the prebuilt binary and a `formae` are present and the MCP is registered in the current harness |
| `/formae:upgrade` | Upgrade local formae when the connected agent is newer (classic mode) — always asks first and warns that it may move a pinned formae |

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

formae-mcp does not resolve the agent endpoint itself. It asks your `formae`
CLI (`formae profile show`) once per tool call, so the MCP and your own
`formae` runs can never disagree about where a profile points. This needs
formae 0.89.0 or newer.

Profiles live at `~/.config/formae/profiles/<name>.pkl` and are managed with
`formae profile` (or the profile tools above); each looks like:

```pkl
amends "formae:/Config.pkl"

cli {
  connection = new Classic {
    url = "http://my-agent-host"
    port = 8080
  }
}
```

A tool's `profile` argument selects which profile that call uses; without one,
formae resolves the active profile. Prefer the per-call argument: the active
pointer is global and shared with your CLI and any other session.

The `FORMAE_AGENT_URL` and `FORMAE_AGENT_PORT` environment variables are no
longer read. Configure a profile instead.

### Which formae the plugin runs

There is exactly one `formae` per machine. On launch the plugin looks for yours
(on `PATH`, then `/opt/pel/bin`, then `/usr/local/bin`) and uses it; only when it
finds none does it download one into `~/.formae-ai/opt`. It never installs a
second copy alongside yours, and it never upgrades an install it did not create —
`/formae:upgrade` tells you which case you are in and, for your own install, gives
you the command to run.

To point the plugin at a specific build, set `FORMAE_BIN` to its path; it is used
verbatim and treated as your own install. `FORMAE_MCP_CHANNEL` (`stable` by
default) selects the channel used when the plugin does have to download.

## License

[FSL-1.1-ALv2](LICENSE)
