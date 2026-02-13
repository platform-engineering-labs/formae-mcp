package tools

// Tool descriptions — these are critical for AI discoverability.
// The AI uses these descriptions to decide when to invoke each tool.

const ListResourcesDescription = `Query infrastructure resources managed by the formae agent. Returns resources with their properties, stack assignment, type, label, and management status.

Use this tool when the user asks about deployed infrastructure, what resources exist, what's in a specific stack, or to find unmanaged resources discovered by the agent.

IMPORTANT: This endpoint returns ALL matching resources with full properties. On large environments a broad query can return hundreds of thousands of characters and overflow the context window. You MUST always combine 'managed:false' with a type filter (e.g., 'managed:false type:AWS::S3::Bucket'). Never use 'managed:false' alone. For broad questions like "what do we have?" or "what's unmanaged?", use get_agent_stats first for an overview of counts by provider, then drill down with type-filtered queries.

Query syntax uses field:value pairs. Supported fields:
- stack: filter by stack name (e.g., 'stack:production')
- type: filter by resource type (e.g., 'type:AWS::S3::Bucket')
- label: filter by resource label (e.g., 'label:my-bucket')
- managed: filter by management status (e.g., 'managed:false' for unmanaged/discovered resources)

Multiple filters can be combined: 'stack:production type:AWS::S3::Bucket'`

const ListStacksDescription = `List all infrastructure stacks known to the formae agent. Returns stack metadata including label, description, and resource count.

Use this tool when the user asks about their stacks, infrastructure organization, or needs an overview of what's deployed.`

const ListTargetsDescription = `Query infrastructure targets (cloud accounts/regions) configured in the formae agent.

Use this tool when the user asks about their cloud targets, configured regions, or provider setup.

Query syntax uses field:value pairs. Supported fields:
- namespace: filter by cloud provider (e.g., 'namespace:AWS')
- discoverable: filter by discovery status (e.g., 'discoverable:true')
- label: filter by target label (e.g., 'label:prod-us-east-1')

Leave query empty to list all targets.`

const GetCommandStatusDescription = `Get the detailed status of a specific formae command by its ID. Returns the command's state, resource updates, and any errors.

Use this tool to check on the progress of a previously submitted apply or destroy command. Commands execute asynchronously in the formae agent.`

const ListCommandsDescription = `List recent formae commands and their statuses. Returns command history with state, timestamps, and resource update summaries.

Use this tool when the user asks about running commands, recent deployments, command history, or what failed.

Query syntax uses field:value pairs. Supported fields:
- id: filter by command ID
- client: filter by client ('client:me' for this session's commands)
- command: filter by type ('command:apply' or 'command:destroy')
- status: filter by state ('status:in_progress', 'status:completed', 'status:failed')
- stack: filter by stack name
- managed: filter by managed status`

const GetAgentStatsDescription = `Get statistics about the formae agent including version, managed/unmanaged resource counts by provider, active plugins, and command counts.

Use this tool to get an overview of the agent's state, check what plugins are loaded, or verify the agent version.`

const CheckHealthDescription = `Check if the formae agent is running and reachable. Returns a simple health status.

Use this tool to verify the agent is available before performing operations.`

const ListPluginsDescription = `List all active formae plugins including resource plugins, schema plugins, and network plugins. Shows plugin name, namespace, version, and capabilities.

Use this tool when the user asks about installed plugins, supported cloud providers, or available resource types.`

const ApplyFormaDescription = `Submit a forma apply command to the formae agent. The command is executed asynchronously — use get_command_status or list_commands to monitor progress.

This tool evaluates the forma file (PKL -> JSON if needed) and submits it to the agent. There are two modes:

- reconcile: Guarantees the target infrastructure matches the forma file exactly. Resources in the file but not deployed are created; deployed resources not in the file are destroyed; differences are updated. This is the default mode for planned deployments.

- patch: Only applies the changes explicitly specified in the forma file. Other resources are untouched. Use this for urgent targeted fixes (e.g., scaling up a cluster during an incident). Patches create drift that should later be reconciled.

Use simulate=true to preview changes without modifying infrastructure.
Use force=true (reconcile only) to overwrite detected drift.

IMPORTANT: Always simulate first and confirm with the user before applying changes to infrastructure.`

const DestroyFormaDescription = `Submit a forma destroy command to remove infrastructure resources. Can destroy by forma file (all resources declared) or by query (matching resources). The command executes asynchronously.

IMPORTANT: Always simulate first and confirm with the user before destroying resources. Destruction is irreversible.`

const CancelCommandsDescription = `Cancel one or more in-progress formae commands. If no query is provided, cancels the most recent in-progress command.

Use this tool when the user wants to stop a running deployment or destroy operation.`

const ForceSyncDescription = `Trigger an immediate synchronization of resource state with the actual cloud infrastructure. The formae agent continuously syncs in the background, but this forces an immediate sync cycle.

Note: In environments with many resources, sync may take significant time. The sync runs asynchronously.`

const ForceDiscoverDescription = `Trigger an immediate resource discovery scan across configured cloud targets. The formae agent discovers new (unmanaged) resources periodically, but this forces an immediate discovery cycle.

Newly discovered resources appear as unmanaged resources that can be queried with list_resources using 'managed:false'.`

const ListChangesSinceLastReconcileDescription = `List infrastructure changes detected since the last reconcile.

Use this tool when the user asks about out-of-band changes, or what has changed in their infrastructure outside of formae since the last reconcile. Returns a list of modified resources grouped by stack, showing the resource type, label, and operation (update/delete).

If a stack is specified, only checks that stack. If no stack is specified, checks all known stacks and aggregates the results.

An empty result means no changes have been detected — the infrastructure matches the last reconciled state.`

const ExtractResourcesDescription = `Extract resources as PKL infrastructure code. Runs 'formae extract' to export matching resources as a PKL forma file that can be incorporated into an IaC codebase.

Use this tool when you need to see the PKL representation of existing resources — typically unmanaged resources that the user wants to bring under formae management. The extracted PKL can then be merged into the user's existing forma files.

The query parameter selects which resources to extract. Always include at least one filter to avoid extracting the entire inventory.

Returns the extracted PKL source code as text.`
