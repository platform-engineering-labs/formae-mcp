package tools

// Tool descriptions — these are critical for AI discoverability.
// The AI uses these descriptions to decide when to invoke each tool.

const ListResourcesDescription = `Query infrastructure resources known to the formae agent, including managed and discovered resources. Returns resources with their properties, stack assignment, type, label, and management status.

In a formae-connected session, use formae inventory first for questions about deployed cloud resources even when the user does not mention formae or name a tool. For example, "which storage buckets do we have in GCP?" calls list_resources with query="type:GCP::Storage::Bucket"; "which S3 buckets?" uses "type:AWS::S3::Bucket". Use list_stacks for stack questions, list_targets for configured cloud accounts/regions, and get_agent_stats before broad resource queries. Apply this routing across providers and both source modes. Questions explicitly about what is declared in a codebase remain source inspection; do not substitute inventory for code or issue cloud queries solely for a source-only question. Carry the selected profile without switching global state. Unless explicitly requested, omit the managed filter so both managed and discovered resources are included. Do not start with gcloud, aws, az or another provider CLI merely because the provider was named. Honor an explicit request for a particular provider tool; otherwise use one only when formae cannot answer, explaining the specific coverage/access/capability gap first. Empty inventory means no matching resources known to that installation; it does not prove the cloud account is empty. Check target coverage and discovery status when relevant, and report unavailable inventory as unavailable rather than zero resources.

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

const ListGeneratorsDescription = `List all generators known to the formae agent. A generator is what draws a credential and, if it declares a cadence, rotates it. Returns each generator's label, stack, type, generation spec, rotation interval, the instant of its last committed rotation, and the resources bound to it.

Use this tool when the user asks what rotates, when something last rotated or will next rotate, which resources take their value from a generator, or whether a credential is generated rather than declared. The drawn value is never returned: only the generation's identity and the destinations it feeds.

An agent older than the generator feature reports none, which is the truthful answer rather than an error.`

const ListTargetsDescription = `Query infrastructure targets (cloud accounts/regions) configured in the formae agent.

Use this tool when the user asks about their cloud targets, configured regions, or provider setup.

Query syntax uses field:value pairs. Supported fields:
- namespace: filter by cloud provider (e.g., 'namespace:AWS')
- discoverable: filter by discovery status (e.g., 'discoverable:true')
- label: filter by target label (e.g., 'label:prod-us-east-1')

Leave query empty to list all targets.`

const GetCommandStatusDescription = `Get the detailed status of a specific formae command by its ID. Returns the command's state, resource updates, and any errors.

Use this tool to check on the progress of a previously submitted apply or destroy command. Commands execute asynchronously in the formae agent.

A resource update in state Rejected means the pre-update cloud read detected an out-of-band change not yet synchronized into inventory. The observed state has been saved and that update was stopped to protect it. The overall command can be Failed without a provider error. Compare refreshed resource state with the forma, re-simulate, and explicitly resolve drift before retrying: use the central absorb/revert review and submission workflow in apply_forma, retaining the original declaration through review and obtaining approval for overwrite. Catch up an explicitly selected maintained project only after central acceptance. A successful simulation does not establish approval; do not rely on a ReconcileRejected response to enforce the decision. Other resources may have succeeded; Failed dependents with empty errors may have been skipped due to the rejection. See formae://docs/troubleshooting.`

const ListCommandsDescription = `List recent formae commands and their statuses. Returns command history with state, timestamps, and resource update summaries.

Use this tool when the user asks about running commands, recent deployments, command history, or what failed.

A Failed command can contain Rejected resource updates: drift protection, not necessarily a provider failure. Use get_command_status to inspect resource states, then re-simulate and handle drift before retrying. See formae://docs/troubleshooting.

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

const ApplyFormaDescription = `Submit a forma apply command to the formae agent. The command is executed asynchronously — use get_command_status or list_commands to monitor progress.

This tool evaluates the forma file (PKL -> JSON if needed) and submits it to the agent. There are two modes:

- reconcile: Guarantees the target infrastructure matches the forma file exactly. Resources in the file but not deployed are created; deployed resources not in the file are destroyed; differences are updated. This is the default mode for planned deployments.

- patch: Only applies the changes explicitly specified in the forma file. Other resources are untouched. Use this for urgent targeted fixes (e.g., scaling up a cluster during an incident). Patches create drift that should later be reconciled.

Ordinary updates use a complete declaration of the affected stack with mode=reconcile and force=false (or omitted), even for one resource or one label. In no-codebase mode, prepare the complete desired stack, modify it, and carry the returned context on both simulation and real submission. In maintained-codebase mode, use the selected complete source and binding context. Small scope, 'quick' wording, a patch-named file, or detected drift does not authorize patch. Use patch only when the user explicitly requests patch mode or their stated incident/hotfix intent calls for a temporary intervention that defers reconciliation. Never switch to patch or force to bypass a drift rejection; follow the explicit keep/revert workflow. If only partial source is available, retrieve the complete desired stack rather than changing the apply mode.

Use simulate=true to preview changes without modifying infrastructure.
For infrastructure apply/authoring, resolve source context before any IaC file search or read: reuse the explicit selection already made for this installation and workspace, or call get_codebase_context with the actual harness working_directory and selected profile. This tool consults the registry; do not replace it with rg/find/glob scans. Mode none is a complete selection, not a missing project to recover. Keep that selection for subsequent operations; do not search again merely because the user asks for another resource change. Resolve again when the installation/workspace changes, the user changes their codebase choice, or a context-validation error requires it. In none mode, locate the affected stack through formae inventory if needed, then prepare_authoring for its complete desired declaration in a fresh disposable directory. Ignore old/unregistered Pkl folders, remembered paths, previous temporary files and Git status. A value in an unselected local file does not establish pending desired intent; inspect formae desired extraction and recorded commands. In codebase mode, read/search only the selected registered root; a missing file or source conflict is an error to resolve there, not permission to search elsewhere. Never register a discovered folder without explicit user opt-in. If selection is required, present registered candidates; do not search for additional candidates. A source-only question about a user-supplied local file can read that exact file without resolving or adopting an infrastructure context; do not broaden that into a project search.

Scratch-project convention: use ~/.formae-ai/scratch/<operation-id>/ for disposable authoring. Resolve the scratch root to a canonical absolute path, create it privately, and create a new unique empty operation directory (for example with mkdtemp); never reuse a fixed main.pkl across operations or search scratch siblings. Pass that exact directory to prepare_authoring, then use its returned file_path, project_path and context to work only inside that operation directory. Additional patch Pkl or evaluated JSON retry files may be created inside that same directory. Keep those paths with the current operation through edit, preview, confirmation, submission and uncertain-outcome retries. A later independent operation creates a fresh directory and extracts desired state again; scratch contents are never the system of record, a Git project, or a codebase registration. Delete only the current operation directory after terminal outcome/retry inspection or abandonment before submission. Do not sweep other sessions or tell no-codebase users about these files unless they ask. If scratch preparation fails, report that concrete failure; do not fall back to home/Library/Documents/Desktop scans or request broad filesystem access to locate IaC. Existing explicit temporary-directory callers remain supported.

Reuse the selected source context, or resolve it with get_codebase_context when none is available for this installation/workspace. For a maintained project pass context {mode:codebase,binding_id}; for no-codebase authoring use prepare_authoring and its full files/dependencies/context. New workflows require the connected agent capabilities, not just a local CLI version.

For ordinary no-codebase changes, extract_resources is the wrong source: it returns PARTIAL ACTUAL inventory, including unabsorbed drift, and can omit failed desired declarations. Never call its output a complete desired stack, even if it contains a Stack object or only one resource is currently visible. Use prepare_authoring with exact stack labels and its complete returned source, dependencies and context. A type-filtered export is for inspection or explicitly selected import, never a shortcut to full-stack reconcile. Do not copy observed OOB values into the prepared desired baseline before the keep/revert review.

Explain changes as keep/revert decisions in either source mode, never infer acceptance from a request to add or edit another property. Read the current drift preference; only auto_absorb_external plus server-proven ExternalChangesOnly=true permits proposing absorb without a new choice. Patches and uncertain history always require a decision; the agent merge must still check conflicts. Resolve actionable drift with all absorb/revert choices keyed by ResourceID: initial soft simulation returns ObservationID; simulate resolution Decisions to obtain the final combined ReviewID; after confirmation submit it with a stable IdempotencyKey. Retain exact input and key for uncertain-outcome retry. stale-review needs a fresh observation, plan and confirmation, never silent force. Pure acceptance is finished centrally by a real command; source catch-up is a separate harness edit using the partial get_command_desired_delta guidance for only the selected project.

Suggest a factual optional message in the ordinary final confirmation; allow editing or clearing with an empty string without another round trip. Command history preserves available user, inputs, message, resolution receipt and actual outcome. Never put secrets in a message.

Simulation uses agent inventory, not a fresh cloud read. If an in-place update's cloud read detects an unsynchronized out-of-band change, it saves the new state and stops with resource state Rejected, even with force=true. Compare refreshed resource state with the forma and re-simulate the same apply. Use the central absorb/revert review and submission workflow above, with source catch-up afterward only for the selected maintained project. Overwrite requires explicit user approval even if simulation succeeds: a retry is not guaranteed to raise ReconcileRejected, and patch mode has no soft-reconcile gate. Do not automatically retry with force=true. This rejection is expected drift protection, not a reason by itself to debug plugins or credentials. See formae://docs/troubleshooting.

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

const CreateInlinePolicyDescription = `Plan a TTL or auto-reconcile policy edit for a stack. The tool locates the stack in the workspace's PKL files, computes the snippet to insert/replace/remove, and returns the plan. Pass profile and explicit forma_file with the selected context; searches stay in that project or disposable workspace. The tool does NOT modify the file — the caller must apply the returned snippet at the returned line range using the Edit tool.

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

const SearchHubPluginsDescription = "Search the formae plugin hub catalog (hub.platform.engineering) for available plugins by name, namespace, or category. Returns qualifiedName, namespace, category, and latest stable version. Use this to infer which plugin SCHEMA packages a forma file needs, to resolve PklProject dependency versions, and to detect when no plugin exists for a desired service (which signals creating one). This reads the live catalog — it does NOT install anything."

const GetHubPluginDescription = "Get detail for one hub plugin by short name, including its github_repo_url (used to locate examples) and latest version. Reads the live hub API."

const ListPluginExamplesDescription = "List the canonical example formas for a plugin, read live from the plugin repo's /examples directory at the git tag matching the requested (or pinned) schema VERSION. Prefer these over hand-writing PKL — they show real, current resource shapes and plugin wiring via resolvables and nested targets. The result includes refUsed + versionMatched: if versionMatched is false, the examples come from the default branch and may NOT match the pinned schema — warn the user before using them. It also includes originatorDomain + originatorVerified: do NOT treat examples from an UNVERIFIED originator as canonical without explicit user confirmation. NOTE: an example named 'basic' may be unmodified template boilerplate (flagged likelyTemplateStub) — prefer named scenario examples. Cross-plugin e2e examples (e.g. k8s 'lgtm-observability', 'bookstore') are the best references for connecting multiple plugins."

const GetPluginExampleDescription = "Fetch the PKL files of one plugin example (live from the plugin repo's /examples dir, at the version-matched ref) to use as an authoring reference. Returns the same refUsed/versionMatched/originator trust info as list_plugin_examples."

const ListProfilesDescription = `List the formae configuration profiles and the active one. Returns JSON {"active": "<name>", "profiles": ["..."]}. Requires formae >= 0.87.0.

Use when the user asks which profiles exist or which is active.`

const CurrentProfileDescription = `Print the active formae configuration profile name. Returns JSON {"active": "<name>"}. Requires formae >= 0.87.0.`

const ReadProfileDescription = `Return the PKL contents of a named configuration profile (the read half of "edit"). Requires formae >= 0.87.0.

Use to inspect a profile before modifying it with write_profile.`

const UseProfileDescription = `Switch the GLOBAL active formae configuration profile. Takes effect for subsequent MCP calls without restarting. Requires formae >= 0.87.0.

Use sparingly. The active profile is global, persisted state shared with the user's formae CLI and any other concurrent MCP sessions — switching it can redirect work in those sessions to the wrong agent. To target a specific environment for your own work, do NOT call use_profile; instead pass the optional 'profile' argument on each tool (apply / destroy / status / inventory / list_* / force_* / cancel / extract). Only call use_profile when the user explicitly asks to change their default environment/agent.`

const SaveProfileDescription = `Snapshot the active profile under a new name (does not switch). Requires formae >= 0.87.0.`

const CreateProfileDescription = `Create a new profile from the starter template (does not switch). Requires formae >= 0.87.0.`

const DeleteProfileDescription = `Delete a profile. Refuses the active profile — switch away first. Requires formae >= 0.87.0.`

const DiffProfilesDescription = `Show a unified diff between two profiles (or <a> vs the active profile). Requires formae >= 0.87.0.`

const WriteProfileDescription = `Overwrite an existing profile's PKL contents (the write half of "edit"). Overwrite-only — use create_profile for new profiles. Refuses the active profile: switch away with use_profile first, or write to a copy. The content is written as-is (formae 0.87.0 has no reliable config validator), so a malformed profile surfaces at next use/apply. Requires formae >= 0.87.0.`

const ActualExtractionNotice = `ACTUAL STATE / PARTIAL SOURCE: this Pkl is a query-scoped inventory export, not the complete desired stack. It can include unabsorbed OOB changes and temporary patches, and omit desired declarations absent from inventory. A stack declaration in this output does not establish completeness, even with a stack-only query. Do not use it as the baseline for an ordinary reconcile or silently carry its observed values into desired state. For an existing-stack edit without a codebase, call prepare_authoring with exact stacks (no type/resource filter), then edit its complete desired source and pass its returned context to apply_forma. A selected maintained codebase remains the source for codebase-mode changes. Use this export for inspection or explicit import; merge only deliberately selected imports into the complete desired workspace. Preserve drift choices for the soft reconcile. In no-codebase mode explain the infrastructure decision, not these internal files.`

const ExtractResourcesDescription = `Export PARTIAL ACTUAL inventory as Pkl for inspection or explicit import. This is not desired-state extraction and is not the authoring entry point for updating an existing stack. For no-codebase edits use prepare_authoring with exact stack labels; for maintained codebases use their selected complete source.

Use this tool to inspect observed resources as Pkl or explicitly import discovered resources. It may contain unabsorbed OOB changes and temporary patches, and may omit failed desired resources. Import only the selected resources into a complete desired workspace or maintained project; do not silently adopt unrelated observed drift.

The query parameter selects which resources to extract. Always include at least one filter to avoid extracting the entire inventory.

Returns an actual-inventory PKL fragment as text for explicit import. It is never a complete desired-stack declaration or acceptance record. Use prepare_authoring for full desired authoring; merge selected import fragments into that complete workspace or the selected maintained project.`

const CreateStandalonePolicyDescription = `Plan the declaration of a standalone (reusable) policy in a forma file. A standalone policy is declared once at the top level of the forma block and can then be attached to any number of stacks with attach_standalone_policy. Use this instead of create_inline_policy when the same policy should govern more than one stack.

Pass profile and explicit forma_file with the selected context; searches stay in that project or disposable workspace. The tool does NOT modify the file — apply the returned snippet at the returned line range using the Edit tool.

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

Pass profile and explicit forma_file with the selected context; searches stay in that project or disposable workspace. The tool does NOT modify the file — apply the returned snippet at the returned line range using the Edit tool, then simulate and apply with apply_forma in reconcile mode.

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

Pass profile and explicit forma_file with the selected context; searches stay in that project or disposable workspace. The tool does NOT modify the file — delete the returned line range using the Edit tool, then simulate and apply with apply_forma in reconcile mode. Detaching does not delete the policy; it stays declared and stays attached to any other stacks.

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

// LoginDescription documents the first half of a hosted sign-in.
const LoginDescription = `Start signing in to the hosted formae platform.

Returns a URL (or a device code) that the user must open themselves — you cannot
complete a sign-in on their behalf. Show it to them, wait, then call
complete_login.

Use this when a hosted profile's session has lapsed, or as part of /formae:setup
on a machine being set up for the hosted platform. It needs no profile and works
on a machine where nothing is configured, which is the case it exists for.

Signing in again while a session is already open is harmless: it reports the
existing session and writes nothing.`

// CompleteLoginDescription documents the second half.
const CompleteLoginDescription = `Finish the sign-in that the login tool started.

Call this after the user says they have finished in the browser (or entered the
device code). It waits for the flow to complete and reports who signed in, the
profiles formae wrote for the installations their grants cover, and which profile
is now active.

Calling it without a preceding login tool call is an error, not a no-op.`

// ConnectCloudAccountDescription is the connect_cloud_account tool description.
//
// The last line is load-bearing. The CLI's own resume command is for a human at
// a terminal, and a model handed a command reads it as an instruction: it will
// run an interactive OAuth or CloudFormation flow in a terminal it cannot drive,
// and burn real state doing so.
const ConnectCloudAccountDescription = "Compute the CloudFormation console link that connects one AWS account to " +
	"the active formae installation. Returns a quick-create URL, the role ARN the stack will produce, and any " +
	"warnings — surface warnings to the user verbatim. This tool changes nothing by itself: the user applies the " +
	"stack in their own browser under their own admin session. Show them the link, wait until they say the stack " +
	"reached CREATE_COMPLETE, then call register_cloud_role. Ask the user for the account id; never infer it from " +
	"ambient credentials. Do NOT run any formae command yourself to do this."

// ListCloudConnectionsDescription is the list_cloud_connections tool
// description.
//
// The "registered" wording is deliberate and matches the tool's own output:
// this reports what the control plane has on file, not whether formae has
// used the role successfully.
const ListCloudConnectionsDescription = "List the cloud accounts this installation has registered. Use this to " +
	"decide whether to offer connect_cloud_account, before starting that flow. An account here is REGISTERED, " +
	"not verified: this does not confirm formae can use the role. If the result says the listing could not be " +
	"determined, treat that as unknown, never as \"no account is registered\": do not send a caller who may " +
	"already have a working connection through the connect flow on that basis."

// RegisterCloudRoleDescription is the register_cloud_role tool description.
//
// It names already_registered as success on purpose: a model that reads the
// idempotent case as a conflict will start trying to repair a connection that is
// already correct.
const RegisterCloudRoleDescription = "Record the role an applied CloudFormation stack produced, completing a cloud " +
	"connection started with connect_cloud_account. Use the expected role ARN that tool reported, unless the user " +
	"says the applied role differs, in which case use theirs. Reporting that the account was already connected with " +
	"the same role is SUCCESS, not a conflict — it is what makes re-running the flow safe. A registered connection " +
	"is complete: there is no verification step to wait for and nothing to poll."

// ListAwsProfilesDescription is the list_aws_profiles tool description.
//
// Showing the account beside the name is the point of this tool: the user is
// picking credentials, not an account, so seeing where each profile resolves
// to is what makes the pick an informed choice about where trust gets
// provisioned. That is why both kinds of row matter and neither is dropped.
const ListAwsProfilesDescription = "List the user's local AWS profiles, each with the account it resolves to, so " +
	"they can pick one to provision a cloud connection with provision_cloud_role. Show every profile: one that " +
	"resolved names its account; one that could not (e.g. an expired SSO session) names the reason instead — that " +
	"is not an error, and it is something the user can act on. Always offer 'none of these' as an option alongside " +
	"the list, which falls back to connect_cloud_account. An empty list is a normal result, not a failure: it means " +
	"connect_cloud_account is the only path available."

// ProvisionCloudRoleDescription is the provision_cloud_role tool description.
//
// The description states plainly what happens and when, matching the
// DestructiveHint annotation this tool carries: unlike connect_cloud_account,
// there is no console step and no user-applied stack standing between this
// call and the mutation.
const ProvisionCloudRoleDescription = "Create the AWS IAM role formae will assume, and register it, using the " +
	"named local AWS profile's credentials — in one call. This happens immediately: it creates a real IAM role, " +
	"and possibly the account-global OIDC identity provider if this account does not have one yet, with no " +
	"console step and nothing for the user to apply themselves. Only call this after list_aws_profiles has shown " +
	"the user the profile and its account, and the user picked it. Reporting that the account was already " +
	"connected with the same role is SUCCESS, not a conflict. A registered connection is complete: there is no " +
	"verification step to wait for and nothing to poll."

// ConnectGcpProjectDescription is the connect_gcp_project tool description.
//
// GCP has one tool where AWS has three, and the description says why: there is
// no console link to hand anyone, so provisioning and registering are a single
// call. It carries the same DestructiveHint as provision_cloud_role for the
// same reason - the mutation happens immediately, with nothing for the user to
// apply themselves.
const ConnectGcpProjectDescription = "Connect a GCP project to the active installation: create the workload " +
	"identity federation formae authenticates through, grant it access to the project, and register the " +
	"connection - in one call. Unlike AWS there is no console link and no stack for the user to apply: GCP has no " +
	"equivalent, so formae does the work with the operator's own Google credentials. This happens immediately and " +
	"grants formae broad access to the project (editor plus the ability to manage the project's IAM), so make sure " +
	"the user means this project before calling. If no usable Google credentials are on the machine, formae signs " +
	"the user in first by running `gcloud auth application-default login`, which opens a browser; tell them to " +
	"expect it. If gcloud is not installed the call fails saying so; the user installs it and you call this tool " +
	"again, and the sign-in happens then - do NOT tell them to run a gcloud login themselves, and do not offer to " +
	"run one for them. If a freshly installed gcloud still reads as missing, their session needs restarting so the " +
	"new PATH is seen. Ask the user " +
	"for the project id and never infer it from gcloud's active configuration. Reporting that the project was " +
	"already connected with the same federation is SUCCESS, not a conflict."

// ConnectAzureSubscriptionDescription is the connect_azure_subscription tool
// description.
//
// Azure has one tool, like GCP, and for the same shape of reason: there is
// one interactive path, so there is nothing to choose between. Unlike GCP,
// formae never spawns a sign-in here - if there are no usable ambient
// credentials the call fails naming the exact `az login` command to run, and
// this tool must relay that command rather than tell the user to sign in
// some other way.
const ConnectAzureSubscriptionDescription = "Connect an Azure subscription to the active installation: create the " +
	"managed identity formae authenticates through, grant it access to the subscription, and register the " +
	"connection - in one call. This happens immediately and grants formae near-owner access to the subscription " +
	"(Contributor plus User Access Administrator), so make sure the user means this subscription before calling. " +
	"formae uses the operator's own ambient Azure credentials (environment variables, managed identity, an " +
	"existing `az login` session); it never opens a browser or spawns a sign-in itself. If there are no usable " +
	"credentials, the call fails naming the exact `az login` command to run - show that command to the user " +
	"verbatim and wait for them to run it themselves, then call this tool again. Ask the user for the " +
	"subscription id and never infer it from ambient credentials or az's active configuration. The tenant id is " +
	"normally derived automatically and should be left unset; only ask the user for it, and pass it as tenant_id, " +
	"when a sign-in attempt already failed reporting no subscriptions found, or the user says their account is a " +
	"guest in another directory or spans several tenants. Reporting that the " +
	"subscription was already connected with the same identity is SUCCESS, not a conflict. There is no " +
	"register-only path through this tool: an operator who will not give it provisioning credentials deploys the " +
	"ARM template themselves and runs `formae connect azure --subscription ... --tenant-id ... --client-id ...` " +
	"in their own terminal - that command is for the user, never for you."

const RegisterAzureTrustDescription = `Register an Azure subscription whose trust was established by deploying the ARM template, instead of having formae provision it. Use this for an operator who will not put provisioning credentials on this machine: they deploy the template in their own portal or pipeline, and this registers the tenant id and client id it outputs. Needs no Azure credential and no az CLI. This validates the coordinates' shape only: it does not check the identity exists, trusts the formae issuer, or grants access.`

const GetAzureTrustTemplateDescription = `Get the one-click Azure portal link that establishes trust without any credential on this machine. Use this when connect_azure_subscription reports no usable Azure credentials, or when the user will not put provisioning credentials here. Returns a portal deep link to show the user: it opens the ARM template with this installation's coordinates already filled in, so there is nothing to paste and no CLI to install. When they report the deployment finished, call register_azure_trust with the tenantId and clientId from its outputs.`

// ListAgentPluginsDescription is the list_agent_plugins tool description.
const ListAgentPluginsDescription = `Report the resource plugins the connected formae installation has installed, and what that implies for authoring.

Call this FIRST when authoring, before search_hub_plugins: it also reports whether the connection is hosted formae or a self-hosted agent, which decides whether the hub catalogue is relevant at all.

On hosted formae the reported set is closed. Plugins ride the agent image and cannot be installed on demand, so a plugin absent from this listing cannot be used, and authoring a new plugin will not make it available. On a self-hosted agent the set is what is installed today, and the user can install more.

For each plugin it names the installed version and the schema package version to pin in PklProject. Those differ: a -dev build publishes its schema over the canonical release coordinate, so pinning the installed version would name a package that has never existed.

Read-only. This lists the AGENT's plugins, which is not what the local ` + "`formae plugin list`" + ` command reports (that one reads this machine's package store).`

const PrepareBugReportDescription = `Prepare an immutable sanitized bug report for hosted formae support, from any workflow. Nothing is submitted. Prefer event_id from the original failed call; only use an explicit hosted profile for an issue outside captured calls and disclose that provenance. Returns the complete preview, report_id and recipient support@formae.ai. Include only relevant local CLI diagnostics; hosted agent logs are already available to support. Never include inventories, full forma files, transcripts or arbitrary log files. Available for this MCP session for one hour. Classic users should use GitHub issues or Discord.`
const SubmitBugReportDescription = `Send a previously prepared bug report to support@formae.ai only with user authorization and confirmed=true. Accepts only report_id and confirmed; submits exactly the sanitized preview to its original hosted installation. Existing explicit session authorization may cover this send. No infrastructure operation is retried. Retry delivery only with the same report_id. A delivery_unknown receipt does not mean email was sent. Prepared reports expire after one hour and are isolated to this MCP session.`
