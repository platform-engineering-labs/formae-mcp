# formae plugin for AI coding assistants

MCP server and skills for [formae](https://github.com/platform-engineering-labs/formae) (formerly distributed as `formae-mcp`). Your AI coding assistant can query, change and discover cloud infrastructure through formae, with self-hosted formae or with [formae Cloud](https://formae.ai/formae-cloud). Works with Claude Code, Codex, Cursor, OpenCode and any other MCP client.

[AI assistants guide](https://docs.formae.ai/documentation/guides/ai-coding-assistants) · [Quick start with an AI assistant](https://docs.formae.ai/documentation/get-started/quickstart-ai-assistant) · [Quick start with formae Cloud](https://docs.formae.ai/documentation/get-started/quickstart-cloud) · [llms.txt](https://docs.formae.ai/llms.txt)

## Prerequisites

- **Self-hosted:** a running formae agent (`formae agent start`) and a formae profile pointing at it.
- **formae Cloud:** a formae Cloud installation.

After installing, run `/formae:setup`: it signs in to formae Cloud or sets up a self-hosted profile, then checks the agent is reachable.

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

## Skills

Ask in plain language; the assistant picks the matching skill. You can also call a skill directly. For example requests, see [Example workflows](https://docs.formae.ai/documentation/guides/ai-coding-assistants#example-workflows); details are in the [MCP tools and skills reference](https://docs.formae.ai/documentation/reference/mcp-tools).

| Skill | Use it to |
|-------|-----------|
| `/formae:formae-author` | Front door for authoring new infrastructure: triages intent, infers plugin dependencies, dispatches to the focused skills below |
| `/formae:formae-project-init` | Start a new formae project from scratch |
| `/formae:formae-deps` | Add or remove a plugin schema dependency in an existing project |
| `/formae:formae-stack-design` | Decide how to group resources into stacks and where to draw boundaries |
| `/formae:formae-apply` | Deploy infrastructure: apply a forma, reconcile or update a stack |
| `/formae:formae-patch` | Urgent, targeted change or incident fix without a full reconcile |
| `/formae:formae-destroy` | Tear down resources, stacks or environments |
| `/formae:formae-rename` | Rename a managed resource without recreating the cloud object |
| `/formae:formae-discover` | Find resources in your cloud accounts that formae does not manage |
| `/formae:formae-import` | Bring discovered resources under formae management |
| `/formae:formae-fix-code-drift` | Check for changes made outside formae and absorb or revert them |
| `/formae:formae-policy` | Set, remove or inspect TTL and auto-reconcile policies on a stack |
| `/formae:formae-secrets` | Generate, rotate and reference credentials such as database passwords and key pairs |
| `/formae:formae-resources` | Inventory questions: which resources exist, by type, stack, label or management status |
| `/formae:formae-stacks` | Overview of stacks with resource counts |
| `/formae:formae-targets` | Configured cloud targets, regions and accounts |
| `/formae:formae-status` | Running commands, deployment progress, history and failures |
| `/formae:formae-connect` | Connect a cloud account to your formae Cloud installation |
| `/formae:formae-config` | Switch, list, save, edit, delete or compare configuration profiles |
| `/formae:formae-plugin-new` | Create a new formae resource plugin for a cloud service or API |
| `/formae:formae-plugin-add-resource` | Add a resource type to an existing plugin |
| `/formae:setup` | Get formae working here: sign in to formae Cloud, or set up a self-hosted profile, then check the agent is reachable |
| `/formae:upgrade` | Upgrade the local formae when the connected agent is newer (self-hosted) |

## MCP tools

The skills are built on the plugin's MCP tools for inventory, apply and destroy, discovery and sync, policies, profiles, hub plugin search, and formae Cloud sign-in and cloud account connections. Your assistant lists them once the plugin is installed; see the [MCP tools and skills reference](https://docs.formae.ai/documentation/reference/mcp-tools) in the docs.

## Configuration

Profiles, the per-call `profile` argument and targeting several environments are described in [Configuration](https://docs.formae.ai/documentation/guides/ai-coding-assistants#configuration). The plugin resolves the agent endpoint through `formae profile show` and needs formae 0.89.0 or newer; the `FORMAE_AGENT_URL` and `FORMAE_AGENT_PORT` environment variables are no longer read.

### Which formae the plugin runs

There is exactly one `formae` per machine. On launch the plugin looks for yours
(on `PATH`, then `/opt/pel/bin`, then `/usr/local/bin`) and uses it; only when it
finds none does it download one into `~/.formae-ai/opt`. It never installs a
second copy alongside yours. A copy it provisioned under `~/.formae-ai/opt` is
silently refreshed on launch; an install it did not create is never upgraded by
the plugin. `/formae:upgrade` tells you which case you are in and, for your own
install, gives you the command to run.

To point the plugin at a specific build, set `FORMAE_BIN` to its path; it is used
verbatim and treated as your own install. `FORMAE_MCP_CHANNEL` (`stable` by
default) selects the channel used when the plugin does have to download.

## License

[FSL-1.1-ALv2](LICENSE)
