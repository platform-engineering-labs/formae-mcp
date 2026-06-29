package tools

// EmptyInput is used for tools that take no parameters.
type EmptyInput struct{}

// ProfileInput carries only the optional per-invocation profile override, for
// tools that otherwise take no parameters.
type ProfileInput struct {
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ListResourcesInput is the input for the list_resources tool.
type ListResourcesInput struct {
	Query   string `json:"query,omitempty" jsonschema:"Bluge query string to filter resources. Supported fields: stack, type, label, managed (boolean). Examples: 'managed:false', 'type:AWS::S3::Bucket stack:production', 'managed:true label:my-bucket'. Leave empty to list all resources."`
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ListTargetsInput is the input for the list_targets tool.
type ListTargetsInput struct {
	Query   string `json:"query,omitempty" jsonschema:"Query string to filter targets. Supported fields: namespace, discoverable, label. Examples: 'namespace:AWS', 'discoverable:true', 'label:prod-us-east-1'. Leave empty to list all targets."`
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// GetCommandStatusInput is the input for the get_command_status tool.
type GetCommandStatusInput struct {
	CommandID string `json:"command_id" jsonschema:"required,The ID of the command to check status for."`
	Profile   string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ListCommandsInput is the input for the list_commands tool.
type ListCommandsInput struct {
	Query      string `json:"query,omitempty" jsonschema:"Query to filter commands. Supported fields: id, client, command (apply/destroy), status (pending/in_progress/completed/failed), stack, managed. Use 'client:me' to filter to your own commands. Leave empty for most recent commands."`
	MaxResults string `json:"max_results,omitempty" jsonschema:"Maximum number of commands to return. Defaults to 10."`
	Profile    string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ApplyFormaInput is the input for the apply_forma tool.
type ApplyFormaInput struct {
	FilePath string `json:"file_path" jsonschema:"required,Absolute path to the forma file (.pkl or .json). PKL files are evaluated locally before submission."`
	Mode     string `json:"mode" jsonschema:"required,Apply mode. 'reconcile': full stack declaration - guarantees infrastructure matches the file exactly. Resources in the file but not in infra are created; resources in infra but not in the file are destroyed; differences are updated. 'patch': only applies the specified changes without affecting other resources - use for targeted urgent fixes."`
	Simulate bool   `json:"simulate,omitempty" jsonschema:"If true, performs a dry-run showing what changes would be made without actually modifying infrastructure. Defaults to false."`
	Force    bool   `json:"force,omitempty" jsonschema:"Only applies to reconcile mode. If true, overwrites any out-of-band changes (drift) detected since the last reconcile. Without force, reconcile rejects if drift is detected."`
	Profile  string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// DestroyFormaInput is the input for the destroy_forma tool.
type DestroyFormaInput struct {
	FilePath string `json:"file_path,omitempty" jsonschema:"Path to the forma file declaring resources to destroy. Mutually exclusive with query."`
	Query    string `json:"query,omitempty" jsonschema:"Query to select resources for destruction. Examples: 'stack:staging', 'type:AWS::S3::Bucket label:temp-data'. Mutually exclusive with file_path."`
	Simulate bool   `json:"simulate,omitempty" jsonschema:"If true, performs a dry-run showing what would be destroyed without actually deleting resources. Defaults to false."`
	Profile  string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// CancelCommandsInput is the input for the cancel_commands tool.
type CancelCommandsInput struct {
	Query   string `json:"query,omitempty" jsonschema:"Optional query to select which commands to cancel. If empty, cancels the most recent in-progress command."`
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ListChangesSinceLastReconcileInput is the input for the list_changes_since_last_reconcile tool.
type ListChangesSinceLastReconcileInput struct {
	Stack   string `json:"stack,omitempty" jsonschema:"Stack label to check for changes. If omitted, checks all stacks."`
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ExtractResourcesInput is the input for the extract_resources tool.
type ExtractResourcesInput struct {
	Query   string `json:"query" jsonschema:"required,Bluge query string to select resources for extraction. Examples: 'managed:false type:AWS::S3::Bucket', 'managed:false stack:production'. Must include at least one filter to avoid extracting all resources."`
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// ForceReconcileStackInput is the input for the force_reconcile_stack tool.
type ForceReconcileStackInput struct {
	Stack   string `json:"stack" jsonschema:"required,The label of the stack to force-reconcile. The stack must have an auto-reconcile policy attached."`
	Profile string `json:"profile,omitempty" jsonschema:"Preferred way to target a named formae environment/agent for THIS call only, without changing global state. Use this in preference to use_profile for per-session targeting: the active profile is global and shared with the user's CLI and any other concurrent sessions, so switching it can hijack work elsewhere. Leave empty to use the active profile. See list_profiles for names. Requires formae >= 0.87.0."`
}

// CreateInlinePolicyInput is the input for the create_inline_policy tool.
type CreateInlinePolicyInput struct {
	Stack           string `json:"stack" jsonschema:"required,The label of the stack to attach the policy to."`
	PolicyType      string `json:"policy_type" jsonschema:"required,The policy type. Must be 'ttl' or 'auto_reconcile'."`
	Operation       string `json:"operation" jsonschema:"required,The operation to perform. Must be 'set' (create or update) or 'remove'."`
	TTLSeconds      int64  `json:"ttl_seconds,omitempty" jsonschema:"Required when policy_type is 'ttl' and operation is 'set'. Time-to-live in seconds. Formatted as a PKL Duration using the largest clean unit (e.g. 1200 -> 20.min, 14400 -> 4.h, 86400 -> 1.d)."`
	OnDependents    string `json:"on_dependents,omitempty" jsonschema:"Optional, only applies to TTL. 'abort' (default) refuses to expire if other stacks depend on this one; 'cascade' destroys dependents too."`
	IntervalSeconds int64  `json:"interval_seconds,omitempty" jsonschema:"Required when policy_type is 'auto_reconcile' and operation is 'set'. Reconcile interval in seconds. Default suggested value is 300 (5 minutes)."`
	FormaFile       string `json:"forma_file,omitempty" jsonschema:"Optional explicit path to the forma file declaring the stack. When omitted the tool searches the workspace using formae eval."`
}

// SearchHubPluginsInput is the input for the search_hub_plugins tool.
type SearchHubPluginsInput struct {
	Query string `json:"query,omitempty" jsonschema:"Optional filter matched against plugin name, namespace, or category (e.g. 'k8s', 'cloud', 'observability'). Leave empty to list the full catalog."`
}

// GetHubPluginInput is the input for the get_hub_plugin tool.
type GetHubPluginInput struct {
	Name string `json:"name" jsonschema:"required,The plugin short name as listed by search_hub_plugins (e.g. 'aws', 'k8s', 'grafana')."`
}

// ListPluginExamplesInput is the input for the list_plugin_examples tool.
type ListPluginExamplesInput struct {
	Plugin  string `json:"plugin" jsonschema:"required,Plugin short name (e.g. 'aws', 'k8s')."`
	Version string `json:"version,omitempty" jsonschema:"Optional plugin schema version to match (e.g. '0.1.5'). Defaults to the project's pinned version or latest. Examples are fetched at the matching git tag; if none exists the default branch is used and versionMatched is false."`
}

// GetPluginExampleInput is the input for the get_plugin_example tool.
type GetPluginExampleInput struct {
	Plugin  string `json:"plugin" jsonschema:"required,Plugin short name."`
	Example string `json:"example" jsonschema:"required,Example name as returned by list_plugin_examples (e.g. 'eks-automode', 'lgtm-observability')."`
	Version string `json:"version,omitempty" jsonschema:"Optional plugin schema version to match (same semantics as list_plugin_examples)."`
}

// ReadProfileInput / Delete/Use share a single required name field; defined per
// tool for clear, specific JSON schemas.
type ReadProfileInput struct {
	Name string `json:"name" jsonschema:"required,The profile name to read."`
}

type UseProfileInput struct {
	Name string `json:"name" jsonschema:"required,The profile name to make active."`
}

type SaveProfileInput struct {
	Name  string `json:"name" jsonschema:"required,The new profile name to snapshot the active profile into."`
	Force bool   `json:"force,omitempty" jsonschema:"Overwrite an existing profile of this name."`
}

type CreateProfileInput struct {
	Name  string `json:"name" jsonschema:"required,The new profile name to create from the starter template (does not switch)."`
	Force bool   `json:"force,omitempty" jsonschema:"Overwrite an existing profile of this name."`
}

type DeleteProfileInput struct {
	Name string `json:"name" jsonschema:"required,The profile name to delete (cannot be the active one)."`
}

type DiffProfilesInput struct {
	A string `json:"a" jsonschema:"required,The first profile name to compare."`
	B string `json:"b,omitempty" jsonschema:"The second profile name; defaults to the active profile when omitted."`
}

type WriteProfileInput struct {
	Name    string `json:"name" jsonschema:"required,The existing, non-active profile to overwrite."`
	Content string `json:"content" jsonschema:"required,The full replacement PKL content for the profile."`
}

// CreateInlinePolicyOutput is the structured response from the create_inline_policy tool.
// The tool does NOT modify the file — the caller (skill / LLM) applies the edit using the Edit tool.
type CreateInlinePolicyOutput struct {
	FilePath              string   `json:"file_path"`
	Operation             string   `json:"operation"`
	PKLSnippet            string   `json:"pkl_snippet,omitempty"`
	InsertionAnchorStart  int      `json:"insertion_anchor_start"`
	InsertionAnchorEnd    int      `json:"insertion_anchor_end"`
	ExistingPolicySnippet string   `json:"existing_policy_snippet,omitempty"`
	ImportsToAdd          []string `json:"imports_to_add,omitempty"`
	Notes                 []string `json:"notes,omitempty"`
}
