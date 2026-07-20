package server

import "fmt"

var serverInstructions = fmt.Sprintf(serverInstructionsTmpl, docsBaseURL)

const serverInstructionsTmpl = `You are connected to a formae MCP server that provides access to a formae infrastructure agent managing cloud resources.

## Key Concepts

- **Forma**: An infrastructure declaration (plural: formae). When applied, formae creates/updates/deletes cloud resources.
- **Stack**: A logical grouping of resources with referential integrity. Default stack is "default". Unmanaged resources live on the "unmanaged" stack.
- **Target**: A cloud account or region where resources are deployed.
- **Resource**: A cloud infrastructure object (S3 bucket, EC2 instance, etc). Can be managed or unmanaged (discovered but not yet under management).

For deeper coverage, read formae://docs/concepts.

## Apply Modes

- **Reconcile** (default): Guarantees infrastructure matches the forma file exactly. Creates missing, destroys extra, updates differences. Use for planned deployments.
- **Patch**: Only applies specified changes. Other resources untouched. Use for urgent fixes during incidents. Creates drift that should later be reconciled.

## Important Workflows

1. **Always simulate before applying**: Use simulate=true to preview changes.
2. **Drift handling**: The agent continuously syncs with cloud state. Drift can be overwritten (force-reconcile) or absorbed.
3. **Discovery**: The agent finds unmanaged resources that can be imported.
4. **Commands are async**: Apply/destroy run asynchronously. Use get_command_status or list_commands to monitor.

## The IaC Language

Formae uses PKL (Apple's configuration language) for forma files. If you need to write or read a forma file:
- PKL basics: formae://docs/pkl-primer (also: https://pkl-lang.org)
- Forma file structure (project, targets, resources blocks): formae://docs/forma-anatomy
- Schema annotations (@formae.ResourceHint, @formae.FieldHint, @formae.Resolvable): formae://docs/annotations

## Policies

Stacks can carry policies that govern their lifecycle:

- **TTL** — destroys the stack after a duration. Fields: ttl, onDependents ("abort" | "cascade").
- **Auto-reconcile** — periodically reverts out-of-band changes. Field: interval.

Policies live in the user's PKL forma files in one of two shapes:

- **Inline** — declared on a single Stack. Plan edits with create_inline_policy.
- **Standalone (reusable)** — declared once at the top level of forma { } and attached to any number of stacks by reference. Plan with create_standalone_policy, attach_standalone_policy, detach_standalone_policy and delete_standalone_policy.

A stack may hold at most one policy per type: it cannot carry both an inline TTL and a standalone TTL. The tools enforce this and refuse with an error naming the conflict.

All of these tools PLAN edits and return a snippet plus a line anchor; they never write files. Apply the plan with Edit, then deploy with apply_forma (or destroy_forma when deleting a standalone). Standalone policies are created and deleted, never updated in place. The /formae-policy skill orchestrates all of this end to end.

## Profiles & targeting (which formae agent a call hits)

A **profile** is a named formae config (endpoint + targets) selecting which agent/environment a command talks to. Requires formae >= 0.87.0.

**Default to the per-invocation ` + "`profile`" + ` argument; do not switch the active profile.** To run a command against a specific environment, pass the optional ` + "`profile`" + ` argument on the tool call. This targets that one call only and changes no global state. It is the correct mechanism for per-session targeting, because the active profile is **global, persisted state shared with the user's CLI and every other concurrent session** — multiple sessions may be working against different agents at once, so calling ` + "`use_profile`" + ` to "set up" your session would silently redirect those other sessions to the wrong agent.

- **Targeting your work** → pass ` + "`profile`" + ` on each call. Never call ` + "`use_profile`" + ` just to prepare a session.
- **` + "`use_profile`" + ` (switching the active profile)** → only when the user **explicitly** asks to change their default environment/agent (e.g. "make prod my default"). It is not a per-session setup step.
- **Which tools accept ` + "`profile`" + `**: the agent-touching tools — apply_forma, destroy_forma, cancel_commands, force_sync, force_discover, force_check_ttl, force_reconcile_stack, list_resources, list_stacks, list_targets, list_policies, list_commands, get_command_status, get_agent_stats, check_health, list_changes_since_last_reconcile, extract_resources. **Do not pass ` + "`profile`" + ` to** the plugin-hub tools (search_hub_plugins, get_hub_plugin, list_plugin_examples, get_plugin_example) or create_inline_policy — they do not support it and the call will be rejected.

## Query Syntax

Queries use field:value pairs separated by spaces (AND-combined). See formae://docs/query-syntax for the full reference.

## Troubleshooting

For common error messages and what they mean: formae://docs/troubleshooting.

## Authoring Infrastructure

Use these tools when helping a user write or scaffold a new plugin or forma project:

- **search_hub_plugins** — full-text search across published hub plugins by keyword.
- **get_hub_plugin** — fetch the full manifest and metadata for a specific hub plugin.
- **list_plugin_examples** — list bundled code examples for a plugin (returns named examples with a likelyTemplateStub flag plus version-match and originator trust info).
- **get_plugin_example** — fetch the source of a specific example file.

Key docs for authoring:
- Forma project layout (main.pkl, modules/, vars.pkl): formae://docs/forma-structure
- Stack design and reconciliation boundaries: formae://docs/stack-design
- Browsable example index: formae://docs/examples
- Common authoring mistakes to avoid: formae://docs/authoring-pitfalls

**Schema vs agent rule**: when authoring a new plugin, the assistant only needs to add the plugin's PKL schema package as a PklProject dependency (no root dependency needed). Resource plugins are compiled Go shared objects installed on the agent machine — the assistant provides guidance only and never installs or builds them. Note: ` + "`formae project init --include <name>`" + ` (non-` + "`@local`" + `) resolves that plugin's version from the agent, so the named plugin must be installed on the agent at init time — or use ` + "`--include <name>@local --plugin-dir <dir>`" + ` to resolve from disk.

## Authoritative Documentation

All formae web documentation lives under %s. Always use complete URLs that include the full path; never invent, shorten, or omit path segments (such as the version prefix). Do not guess documentation URLs — read formae://docs/index for the canonical list of pages, or use the formae://docs/* resources directly.
`
