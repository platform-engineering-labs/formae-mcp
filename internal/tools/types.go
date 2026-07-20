package tools

// EmptyInput is used for tools that take no parameters.
type EmptyInput struct{}

// ListResourcesInput is the input for the list_resources tool.
type ListResourcesInput struct {
	Query string `json:"query,omitempty" jsonschema:"Bluge query string to filter resources. Supported fields: stack, type, label, managed (boolean). Examples: 'managed:false', 'type:AWS::S3::Bucket stack:production', 'managed:true label:my-bucket'. Leave empty to list all resources."`
}

// ListTargetsInput is the input for the list_targets tool.
type ListTargetsInput struct {
	Query string `json:"query,omitempty" jsonschema:"Query string to filter targets. Supported fields: namespace, discoverable, label. Examples: 'namespace:AWS', 'discoverable:true', 'label:prod-us-east-1'. Leave empty to list all targets."`
}

// GetCommandStatusInput is the input for the get_command_status tool.
type GetCommandStatusInput struct {
	CommandID string `json:"command_id" jsonschema:"required,The ID of the command to check status for."`
}

// ListCommandsInput is the input for the list_commands tool.
type ListCommandsInput struct {
	Query      string `json:"query,omitempty" jsonschema:"Query to filter commands. Supported fields: id, client, command (apply/destroy), status (pending/in_progress/completed/failed), stack, managed. Use 'client:me' to filter to your own commands. Leave empty for most recent commands."`
	MaxResults string `json:"max_results,omitempty" jsonschema:"Maximum number of commands to return. Defaults to 10."`
}

// ApplyFormaInput is the input for the apply_forma tool.
type ApplyFormaInput struct {
	FilePath string `json:"file_path" jsonschema:"required,Absolute path to the forma file (.pkl or .json). PKL files are evaluated locally before submission."`
	Mode     string `json:"mode" jsonschema:"required,Apply mode. 'reconcile': full stack declaration - guarantees infrastructure matches the file exactly. Resources in the file but not in infra are created; resources in infra but not in the file are destroyed; differences are updated. 'patch': only applies the specified changes without affecting other resources - use for targeted urgent fixes."`
	Simulate bool   `json:"simulate,omitempty" jsonschema:"If true, performs a dry-run showing what changes would be made without actually modifying infrastructure. Defaults to false."`
	Force    bool   `json:"force,omitempty" jsonschema:"Only applies to reconcile mode. If true, overwrites any out-of-band changes (drift) detected since the last reconcile. Without force, reconcile rejects if drift is detected."`
}

// DestroyFormaInput is the input for the destroy_forma tool.
type DestroyFormaInput struct {
	FilePath string `json:"file_path,omitempty" jsonschema:"Path to the forma file declaring resources to destroy. Mutually exclusive with query."`
	Query    string `json:"query,omitempty" jsonschema:"Query to select resources for destruction. Examples: 'stack:staging', 'type:AWS::S3::Bucket label:temp-data'. Mutually exclusive with file_path."`
	Simulate bool   `json:"simulate,omitempty" jsonschema:"If true, performs a dry-run showing what would be destroyed without actually deleting resources. Defaults to false."`
}

// CancelCommandsInput is the input for the cancel_commands tool.
type CancelCommandsInput struct {
	Query string `json:"query,omitempty" jsonschema:"Optional query to select which commands to cancel. If empty, cancels the most recent in-progress command."`
}

// ListChangesSinceLastReconcileInput is the input for the list_changes_since_last_reconcile tool.
type ListChangesSinceLastReconcileInput struct {
	Stack string `json:"stack,omitempty" jsonschema:"Stack label to check for changes. If omitted, checks all stacks."`
}

// ExtractResourcesInput is the input for the extract_resources tool.
type ExtractResourcesInput struct {
	Query string `json:"query" jsonschema:"required,Bluge query string to select resources for extraction. Examples: 'managed:false type:AWS::S3::Bucket', 'managed:false stack:production'. Must include at least one filter to avoid extracting all resources."`
}

// ForceReconcileStackInput is the input for the force_reconcile_stack tool.
type ForceReconcileStackInput struct {
	Stack string `json:"stack" jsonschema:"required,The label of the stack to force-reconcile. The stack must have an auto-reconcile policy attached."`
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

// CreateStandalonePolicyInput is the input for the create_standalone_policy tool.
type CreateStandalonePolicyInput struct {
	Label           string `json:"label" jsonschema:"required,The label for the new standalone policy. This is how stacks reference it, so it must be unique across the project. Prefer descriptive labels like 'ephemeral-1h' or 'nightly-drift'."`
	PolicyType      string `json:"policy_type" jsonschema:"required,The policy type. Must be 'ttl' or 'auto_reconcile'."`
	TTLSeconds      int64  `json:"ttl_seconds,omitempty" jsonschema:"Required when policy_type is 'ttl'. Time-to-live in seconds. Formatted as a PKL Duration using the largest clean unit (e.g. 1200 -> 20.min, 14400 -> 4.h, 86400 -> 1.d)."`
	OnDependents    string `json:"on_dependents,omitempty" jsonschema:"Optional, only applies to TTL. 'abort' (default) refuses to expire if other stacks depend on this one; 'cascade' destroys dependents too."`
	IntervalSeconds int64  `json:"interval_seconds,omitempty" jsonschema:"Required when policy_type is 'auto_reconcile'. Reconcile interval in seconds. Default suggested value is 300 (5 minutes)."`
	FormaFile       string `json:"forma_file,omitempty" jsonschema:"Optional explicit path to the forma file that should carry the declaration. When omitted the tool picks the workspace's main forma file (the one declaring the most stacks) and errors if there is no single winner."`
}

// CreateStandalonePolicyOutput describes the planned edit. The tool does NOT
// modify the file — the caller applies the edit using the Edit tool.
type CreateStandalonePolicyOutput struct {
	FilePath             string   `json:"file_path"`
	Operation            string   `json:"operation"`
	PKLSnippet           string   `json:"pkl_snippet,omitempty"`
	InsertionAnchorStart int      `json:"insertion_anchor_start"`
	InsertionAnchorEnd   int      `json:"insertion_anchor_end"`
	ImportsToAdd         []string `json:"imports_to_add,omitempty"`
	Notes                []string `json:"notes,omitempty"`
}

// AttachStandalonePolicyInput is the input for the attach_standalone_policy tool.
type AttachStandalonePolicyInput struct {
	Stack       string `json:"stack" jsonschema:"required,The label of the stack to attach the policy to."`
	PolicyLabel string `json:"policy_label" jsonschema:"required,The label of the existing standalone policy to attach."`
	FormaFile   string `json:"forma_file,omitempty" jsonschema:"Optional explicit path to the forma file declaring the stack. When omitted the tool searches the workspace using formae eval."`
}

// AttachStandalonePolicyOutput describes the planned edit. The tool does NOT
// modify the file — the caller applies the edit using the Edit tool.
type AttachStandalonePolicyOutput struct {
	FilePath             string   `json:"file_path"`
	Operation            string   `json:"operation"`
	PKLSnippet           string   `json:"pkl_snippet,omitempty"`
	InsertionAnchorStart int      `json:"insertion_anchor_start"`
	InsertionAnchorEnd   int      `json:"insertion_anchor_end"`
	ImportsToAdd         []string `json:"imports_to_add,omitempty"`
	Notes                []string `json:"notes,omitempty"`
}

// DetachStandalonePolicyInput is the input for the detach_standalone_policy tool.
type DetachStandalonePolicyInput struct {
	Stack       string `json:"stack" jsonschema:"required,The label of the stack to detach the policy from."`
	PolicyLabel string `json:"policy_label" jsonschema:"required,The label of the standalone policy to detach."`
	FormaFile   string `json:"forma_file,omitempty" jsonschema:"Optional explicit path to the forma file declaring the stack. When omitted the tool searches the workspace using formae eval."`
}

// DetachStandalonePolicyOutput describes the planned edit. The tool does NOT
// modify the file — the caller applies the edit using the Edit tool.
type DetachStandalonePolicyOutput struct {
	FilePath                  string   `json:"file_path"`
	Operation                 string   `json:"operation"`
	SourceAnchorStart         int      `json:"source_anchor_start"`
	SourceAnchorEnd           int      `json:"source_anchor_end"`
	ExistingResolvableSnippet string   `json:"existing_resolvable_snippet,omitempty"`
	Notes                     []string `json:"notes,omitempty"`
}

// DeleteStandalonePolicyInput is the input for the delete_standalone_policy tool.
type DeleteStandalonePolicyInput struct {
	Label string `json:"label" jsonschema:"required,The label of the standalone policy to delete. The policy must not be attached to any stack — detach it everywhere first."`
}

// DeleteStandalonePolicyOutput describes the planned deletion. The tool does
// NOT modify anything — the caller removes the source lines with Edit, then
// writes destroy_forma_pkl to a temp file and calls destroy_forma on it.
type DeleteStandalonePolicyOutput struct {
	FilePath              string   `json:"file_path"`
	Operation             string   `json:"operation"`
	SourceAnchorStart     int      `json:"source_anchor_start"`
	SourceAnchorEnd       int      `json:"source_anchor_end"`
	ExistingPolicySnippet string   `json:"existing_policy_snippet,omitempty"`
	DestroyFormaPKL       string   `json:"destroy_forma_pkl,omitempty"`
	Notes                 []string `json:"notes,omitempty"`
}
