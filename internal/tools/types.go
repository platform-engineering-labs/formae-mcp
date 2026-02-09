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

// ExtractResourcesInput is the input for the extract_resources tool.
type ExtractResourcesInput struct {
	Query string `json:"query" jsonschema:"required,Bluge query string to select resources for extraction. Examples: 'managed:false type:AWS::S3::Bucket', 'managed:false stack:production'. Must include at least one filter to avoid extracting all resources."`
}
