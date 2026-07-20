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

const ListPoliciesDescription = `List all standalone (reusable) policies known to the formae agent. Returns each policy's label, type ("ttl" or "auto-reconcile"), configuration, and the stacks it is attached to.

Use this tool when the user asks about reusable policies, which stacks share a policy, or what standalone policies exist. For inline policies attached directly to a stack, use list_stacks — inline policies appear on each stack object.`

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

const ForceCheckTTLDescription = `Trigger an immediate TTL expiry check across all stacks. Stacks whose TTL policy has expired will be destroyed asynchronously. Stacks with active commands are skipped. The agent runs this check periodically; this tool forces an immediate sweep.

DESTRUCTIVE: This tool destroys infrastructure for any stack whose TTL has expired. The destruction is irreversible.

Primarily useful for test harnesses and incident response. For normal operation the agent runs this automatically.`

const ForceReconcileStackDescription = `Force a one-shot reconcile on a specific stack. Reverts any out-of-band changes to managed resources on the stack back to their last-known desired state. The stack must have an auto-reconcile policy attached.

Returns 202 with a command_id when the reconcile starts (poll get_command_status to monitor progress). Returns 200 if there is no drift to reconcile. Returns 403 if the stack has no auto-reconcile policy attached. Returns 409 if the stack has active commands.

Primarily useful for test harnesses and incident response. For normal operation the agent runs scheduled reconciles based on the policy interval.`

const CreateInlinePolicyDescription = `Plan a TTL or auto-reconcile policy edit for a stack. The tool locates the stack in the workspace's PKL files, computes the snippet to insert/replace/remove, and returns the plan. The tool does NOT modify the file — the caller must apply the returned snippet at the returned line range using the Edit tool.

Output fields:
- file_path: which PKL file declares the stack
- operation: "create" (new policy added), "update" (existing policy of same type replaced), "remove" (policy deleted), or "noop" (remove requested but no matching policy existed)
- pkl_snippet: the text to insert (empty for remove and noop)
- insertion_anchor_start / insertion_anchor_end: 1-indexed inclusive line range; for "create" with start == end the snippet should be inserted before that line; for "update" / "remove" the lines in the range should be replaced/deleted
- existing_policy_snippet: the existing block being replaced or deleted (only for update/remove)
- imports_to_add: list of import statements that must be added at the top of the file (e.g. import "@formae/formae.pkl")
- notes: human-readable observations (e.g. "removed empty policies block")

After applying the edit, run apply_forma in reconcile mode (simulate=true first, then simulate=false on confirmation) on the returned file_path.

Errors when: the stack is unknown, no PKL file in the workspace declares it, or multiple files declare the same label.

Refuses when the stack already has a standalone (reusable) policy of the same type attached — a stack may hold only one policy per type. The error names the standalone; detach it with detach_standalone_policy, or update the standalone instead of setting an inline policy. This check runs only for operation="set".
`

const ListChangesSinceLastReconcileDescription = `List infrastructure changes detected since the last reconcile.

Use this tool when the user asks about out-of-band changes, or what has changed in their infrastructure outside of formae since the last reconcile. Returns a list of modified resources grouped by stack, showing the resource type, label, and operation (update/delete).

If a stack is specified, only checks that stack. If no stack is specified, checks all known stacks and aggregates the results.

An empty result means no changes have been detected — the infrastructure matches the last reconciled state.`

const ExtractResourcesDescription = `Extract resources as PKL infrastructure code. Runs 'formae extract' to export matching resources as a PKL forma file that can be incorporated into an IaC codebase.

Use this tool when you need to see the PKL representation of existing resources — typically unmanaged resources that the user wants to bring under formae management. The extracted PKL can then be merged into the user's existing forma files.

The query parameter selects which resources to extract. Always include at least one filter to avoid extracting the entire inventory.

Returns the extracted PKL source code as text.`

const CreateStandalonePolicyDescription = `Plan the declaration of a standalone (reusable) policy in a forma file. A standalone policy is declared once at the top level of the forma block and can then be attached to any number of stacks with attach_standalone_policy. Use this instead of create_inline_policy when the same policy should govern more than one stack.

The tool does NOT modify the file — apply the returned snippet at the returned line range using the Edit tool.

Output fields:
- file_path: the forma file that should carry the declaration (the workspace's main forma file unless forma_file was given)
- operation: "create", or "noop" when a standalone with that label already exists
- pkl_snippet: the declaration to insert
- insertion_anchor_start / insertion_anchor_end: 1-indexed inclusive line range; these are equal, and the snippet is inserted BEFORE that line (the closing brace of the forma block)
- imports_to_add: import statements to add at the top of the file if missing
- notes: human-readable observations

Creating a standalone policy attaches it to nothing and changes no infrastructure on its own. Follow up with attach_standalone_policy for each stack that should carry it, then apply.

Errors when: no single main forma file can be identified (pass forma_file to disambiguate), the target file has no forma block, or the project pins a formae PKL schema older than 0.82.0 (policies did not exist yet).`

const AttachStandalonePolicyDescription = `Plan the attachment of an existing standalone (reusable) policy to a stack. Inserts a PolicyResolvable reference into the stack's policies listing, creating the listing if the stack has none.

The tool does NOT modify the file — apply the returned snippet at the returned line range using the Edit tool, then simulate and apply with apply_forma in reconcile mode.

Output fields:
- file_path: the PKL file declaring the stack
- operation: "attach", or "noop" when this policy is already attached to this stack
- pkl_snippet: the entry to insert (wrapped in policies = new Listing { ... } when the stack had no listing)
- insertion_anchor_start / insertion_anchor_end: 1-indexed inclusive line range; these are equal and the snippet is inserted BEFORE that line
- imports_to_add: import statements to add at the top of the file if missing
- notes: human-readable observations

Hard-refuses when the stack already carries an inline policy of the same type, or a different standalone of the same type — a stack may hold only one policy per type. The error names the conflicting policy so it can be removed or detached first.

Errors when: the standalone policy is unknown to the agent, the stack is unknown, no PKL file declares the stack, several files declare it, or the project pins a formae PKL schema older than 0.82.0.`

const DetachStandalonePolicyDescription = `Plan the detachment of a standalone (reusable) policy from a stack. Locates the PolicyResolvable entry in the stack's policies listing — both the direct 'new formae.PolicyResolvable { label = "X" }' form and the '<binding>.res' form are recognised — and returns the line range to delete.

The tool does NOT modify the file — delete the returned line range using the Edit tool, then simulate and apply with apply_forma in reconcile mode. Detaching does not delete the policy; it stays declared and stays attached to any other stacks.

Output fields:
- file_path: the PKL file declaring the stack
- operation: "detach", or "noop" when the policy is not attached to this stack
- source_anchor_start / source_anchor_end: 1-indexed inclusive line range to DELETE
- existing_resolvable_snippet: the text being removed, for the diff
- notes: human-readable observations; includes "removed empty policies block" when the entry was the listing's only member, in which case the anchor covers the whole policies = new Listing { ... } wrapper

Errors when: the stack is unknown, no PKL file declares it, or several files declare it.`

const DeleteStandalonePolicyDescription = `Plan the deletion of a standalone (reusable) policy. Refuses while the policy is still attached to any stack.

The tool does NOT modify anything. Applying the plan is a two-step sequence and THE ORDER MATTERS:
1. Delete source_anchor_start..source_anchor_end from file_path with the Edit tool.
2. Write destroy_forma_pkl to a temporary file and call destroy_forma on it (simulate first, then for real).

Source edit BEFORE destroy: if a reconcile happens between the two steps the agent sees no policy in any forma and does nothing. In the reverse order a reconcile in between would recreate the policy.

Output fields:
- file_path: the PKL file declaring the standalone policy
- operation: "delete"
- source_anchor_start / source_anchor_end: 1-indexed inclusive line range to DELETE
- existing_policy_snippet: the declaration being removed, for the diff
- destroy_forma_pkl: a complete standalone forma declaring only this policy, rendered from the agent's stored config — write it to a temp file and pass it to destroy_forma
- notes: human-readable observations, including a warning when the declaration is bound to a PKL local and leaves references behind

Hard-refuses when the policy is still attached to one or more stacks; the error lists them. Detach it from each (detach_standalone_policy) and apply those changes first.

If destroy_forma later returns a Skip operation with ReferencingStacks, someone attached the policy between the pre-check and the destroy: the source is already edited but the policy still exists in the agent. Report that plainly and name the attaching stacks.

Errors when: the policy is unknown to the agent, or its source declaration cannot be located in the workspace.`
