package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) registerResources() {
	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/query-syntax",
		Name:        "Formae Query Syntax",
		Description: "Reference documentation for formae's Bluge-based query syntax used to filter resources, targets, and commands.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/query-syntax",
				Text: querySyntaxDoc,
			}},
		}, nil
	})

	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/concepts",
		Name:        "Formae Core Concepts",
		Description: "Overview of formae's core concepts: stacks, targets, resources, formas, modes, drift, and discovery.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/concepts",
				Text: conceptsDoc,
			}},
		}, nil
	})
}

const querySyntaxDoc = `# Formae Query Syntax

Formae uses a Bluge-based query syntax for filtering resources, targets, and commands.

## Format

Queries use field:value pairs separated by spaces. Multiple pairs are AND-combined.

## Resource Query Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| stack | string | Stack name | stack:production |
| type | string | Resource type | type:AWS::S3::Bucket |
| label | string | Resource label | label:my-bucket |
| managed | boolean | Management status | managed:false |

## Target Query Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| namespace | string | Cloud provider | namespace:AWS |
| discoverable | boolean | Discovery enabled | discoverable:true |
| label | string | Target label | label:prod-us-east-1 |

## Command Query Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| id | string | Command ID | id:abc123 |
| client | string | Client ID | client:me |
| command | string | Command type | command:apply |
| status | string | Command state | status:in_progress |
| stack | string | Stack name | stack:production |
| managed | boolean | Managed status | managed:true |

## Examples

- All unmanaged resources: managed:false
- S3 buckets in production: type:AWS::S3::Bucket stack:production
- Failed commands: status:failed
- Running commands: status:in_progress
`

const conceptsDoc = `# Formae Core Concepts

## Architecture

Formae uses a client-server architecture. The CLI is the client, and the formae
agent runs as a server in or near the user's infrastructure. The agent
continuously synchronizes with cloud providers to maintain an up-to-date view of
infrastructure state.

## Forma

A forma (plural: formae) is an infrastructure declaration. When you apply a
forma, formae processes it to create, update, or delete resources. A forma can
target an entire environment or a single resource change.

## Stack

A stack is a logical grouping of resources with referential integrity. Destroying
one resource in a stack may cascade to dependent resources. The default stack is
named "default". Unmanaged resources discovered by the agent live on a special
unmanaged stack.

## Target

A target represents a cloud account or region where resources are deployed. Each
target has a namespace (e.g., AWS, Azure) and configuration (e.g., region,
credentials).

## Resource

A resource represents a cloud infrastructure object (e.g., an S3 bucket, an EC2
instance). Resources can be managed (declared in a forma and actively managed)
or unmanaged (discovered by the agent but not yet under management).

## Apply Modes

### Reconcile Mode (default)
Guarantees the target infrastructure matches the forma file exactly:
- Resources in the file but not deployed are created
- Deployed resources not in the file are destroyed
- Differences between file and deployed state are updated

### Patch Mode
Only applies the changes explicitly specified in the forma. Other resources are
untouched. Use for urgent targeted fixes. Patches create drift that should later
be reconciled.

## Simulation
Both apply and destroy support a simulate flag for dry-run previews. Always
simulate before applying changes.

## Drift

Drift occurs when infrastructure state diverges from the declared state. Sources:
- **Sync drift**: Out-of-band changes made directly in the cloud console or by
  other tools, detected by the agent's continuous synchronization.
- **Patch drift**: Changes applied via patch mode that haven't been reconciled
  into a full stack declaration.

Users can handle drift by either:
- **Overwriting**: Force-reconciling to restore the declared state
- **Absorbing**: Incorporating the drift into their IaC codebase

## Discovery

The agent periodically scans cloud accounts for resources not managed by formae.
Discovered resources appear as unmanaged and can be queried, inspected, and
optionally imported under management.

## Commands

Apply and destroy operations execute as asynchronous commands in the agent.
Commands have states: pending, in_progress, completed, failed, canceled. Use
command status queries to monitor progress.
`
