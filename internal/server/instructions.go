package server

const serverInstructions = `You are connected to a formae MCP server that provides access to a formae infrastructure agent managing cloud resources.

## Key Concepts

- **Forma**: An infrastructure declaration (plural: formae). When applied, formae creates/updates/deletes cloud resources.
- **Stack**: A logical grouping of resources with referential integrity. Default stack is "default". Unmanaged resources live on the "unmanaged" stack.
- **Target**: A cloud account or region where resources are deployed.
- **Resource**: A cloud infrastructure object (S3 bucket, EC2 instance, etc). Can be managed or unmanaged (discovered but not yet under management).

## Apply Modes

- **Reconcile** (default): Guarantees infrastructure matches the forma file exactly. Creates missing resources, destroys extra resources, updates differences. Use for planned deployments.
- **Patch**: Only applies specified changes. Other resources are untouched. Use for urgent fixes during incidents. Creates drift that should later be reconciled.

## Important Workflows

1. **Always simulate before applying**: Use simulate=true to preview changes before modifying infrastructure.
2. **Drift handling**: The agent continuously syncs with cloud state. Drift can be overwritten (force-reconcile) or absorbed into the IaC codebase.
3. **Discovery**: The agent discovers unmanaged resources that can be imported under management.
4. **Commands are async**: Apply and destroy operations run asynchronously. Use get_command_status or list_commands to monitor progress.

## Query Syntax

Queries use field:value pairs separated by spaces (AND-combined). Read the formae://docs/query-syntax resource for full reference.
`
