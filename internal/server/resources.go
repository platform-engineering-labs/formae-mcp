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

	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/pkl-primer",
		Name:        "PKL Primer for Formae",
		Description: "Minimal PKL syntax reference for reading and writing forma files. Covers modules, imports, object literals, mappings, Resolvables, and hidden fields.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/pkl-primer",
				Text: pklPrimerDoc,
			}},
		}, nil
	})

	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/forma-anatomy",
		Name:        "Forma File Anatomy",
		Description: "Structure of a forma file: top-level fields, project layout, cross-resource references, and labels.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/forma-anatomy",
				Text: formaAnatomyDoc,
			}},
		}, nil
	})

	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/annotations",
		Name:        "Formae Schema Annotations",
		Description: "PKL annotations used by plugin authors: ResourceHint, FieldHint, ConfigFieldHint, Resolvable, SubResourceHint.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/annotations",
				Text: annotationsDoc,
			}},
		}, nil
	})

	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/troubleshooting",
		Name:        "Formae Troubleshooting",
		Description: "Common formae error messages and what they mean: plugin-not-found, drift, createOnly replacement, stuck commands, discovery issues.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/troubleshooting",
				Text: troubleshootingDoc,
			}},
		}, nil
	})

	s.mcpServer.AddResource(&mcp.Resource{
		URI:         "formae://docs/index",
		Name:        "Formae Documentation Index",
		Description: "Canonical list of formae documentation URLs. Read this to find the correct doc link instead of constructing or guessing one.",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:  "formae://docs/index",
				Text: docsIndexDoc(),
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

Canonical reference: https://docs.formae.io/en/latest/core-concepts/forma/

## Stack

A stack is a logical grouping of resources with referential integrity. Destroying
one resource in a stack may cascade to dependent resources. The default stack is
named "default". Unmanaged resources discovered by the agent live on a special
unmanaged stack.

Canonical reference: https://docs.formae.io/en/latest/core-concepts/stack/

## Target

A target represents a cloud account or region where resources are deployed. Each
target has a namespace (e.g., AWS, Azure) and configuration (e.g., region,
credentials).

Canonical reference: https://docs.formae.io/en/latest/core-concepts/target/

## Resource

A resource represents a cloud infrastructure object (e.g., an S3 bucket, an EC2
instance). Resources can be managed (declared in a forma and actively managed)
or unmanaged (discovered by the agent but not yet under management).

Canonical reference: https://docs.formae.io/en/latest/core-concepts/properties/

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

Canonical reference: https://docs.formae.io/en/latest/core-concepts/apply-modes/

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

Canonical reference: https://docs.formae.io/en/latest/core-concepts/synchronization/

## Discovery

The agent periodically scans cloud accounts for resources not managed by formae.
Discovered resources appear as unmanaged and can be queried, inspected, and
optionally imported under management.

Canonical reference: https://docs.formae.io/en/latest/core-concepts/discovery/

## Commands

Apply and destroy operations execute as asynchronous commands in the agent.
Commands have states: pending, in_progress, completed, failed, canceled. Use
command status queries to monitor progress.

## Going deeper

- Core concepts index: https://docs.formae.io/en/latest/core-concepts/
- Plugin concept: https://docs.formae.io/en/latest/core-concepts/plugin/
- Values and properties: https://docs.formae.io/en/latest/core-concepts/values/
`

const pklPrimerDoc = `# PKL Primer for Formae

Formae uses PKL (https://pkl-lang.org) — Apple's configuration language — as its
primary IaC syntax. PKL is to formae what HCL is to Terraform.

## Why PKL

PKL is a typed configuration language that combines the declarative feel of YAML
with the safety of a type system, the composability of a real programming
language, and excellent IDE support. Forma files are PKL modules.

## Minimum PKL you need to know

### Modules and imports

A PKL file is a module. The top of a forma file typically looks like:

` + "```pkl" + `
amends "@formae/forma.pkl"

import "@aws/aws.pkl" as aws
import "@aws/s3/bucket.pkl"
` + "```" + `

- ` + "`amends`" + ` means "extend this base module's schema." Forma files always amend the formae forma module.
- ` + "`import \"@<plugin>/...\" as <alias>`" + ` brings a plugin's schema into scope.
- Package URIs like ` + "`@aws/`" + ` are resolved via the project's PklProject file.

### Object literals

PKL objects use ` + "`new <Type> { field = value }`" + `:

` + "```pkl" + `
new bucket.Bucket {
  label = "my-app-assets"
  bucketName = "my-app-assets-prod"
  versioningEnabled = true
}
` + "```" + `

### Mappings and listings

` + "```pkl" + `
tags = new Mapping<String, String> {
  ["Environment"] = "production"
  ["Owner"] = "platform-team"
}

allowedOrigins = new Listing<String> {
  "https://app.example.com"
}
` + "```" + `

### Late binding (Resolvables)

Some fields can hold a Resolvable — a reference to another resource's output
that gets evaluated at apply time:

` + "```pkl" + `
new lambda.Function {
  label = "api-handler"
  // Resolves at apply time to the bucket's actual ARN
  environment = new Mapping<String, String|formae.Resolvable> {
    ["BUCKET_ARN"] = bucket.res.arn
  }
}
` + "```" + `

The ` + "`.res`" + ` accessor and resolvable fields are defined by each resource's
schema. See https://docs.formae.io/en/latest/core-concepts/res/.

### Hidden vs public fields

` + "`hidden`" + ` fields aren't serialized to the on-the-wire form. They exist for
PKL-local convenience (typing, defaults). Public fields (no ` + "`hidden`" + `)
become part of the forma's serialized representation.

## What you don't need to know

PKL has classes, methods, type aliases, generators, and a lot more. For forma
files you mostly need: amends, imports, object literals, mappings, listings.
The schema does the heavy lifting.

## Going deeper

- Formae PKL cheatsheet: https://docs.formae.io/en/latest/pkl-cheatsheet/
- Official PKL site: https://pkl-lang.org/
- PKL language reference: https://pkl-lang.org/main/current/language-reference/
- Forma file structure: formae://docs/forma-anatomy
`

const formaAnatomyDoc = `# Forma File Anatomy

A forma is a PKL module that declares infrastructure. The agent reads it,
computes a changeset, and applies the changes.

## Minimal forma

` + "```pkl" + `
amends "@formae/forma.pkl"
import "@formae/formae.pkl"
import "@aws/aws.pkl"
import "@aws/s3/bucket.pkl"

local assets = new bucket.Bucket {
  label = "my-app-assets"
  bucketName = "my-app-assets-prod"
}

forma {
  new formae.Stack { label = "default" }
  new formae.Target {
    label = "aws"
    config = new aws.Config { region = "us-west-2" }
  }
  assets
}
` + "```" + `

## The ` + "`forma {}`" + ` block

The ` + "`forma {}`" + ` block is the top-level declaration. It contains three kinds of
elements in any order:

- **` + "`new formae.Stack`" + `** — declares the logical stack that groups these resources.
  Typically one per file; the default stack name is "default".
- **` + "`new formae.Target`" + `** — declares a deployment target (cloud account + region +
  credentials). One or more targets can appear.
- **Resources** — instances of plugin-defined types (` + "`bucket.Bucket`" + `,
  ` + "`function.Function`" + `, etc.) or spread from a sibling module via
  ` + "`...module.resources`" + `.

**Single-target rule**: when there is exactly one target in the block, all
resources are implicitly deployed to it. With multiple targets, each resource
must set ` + "`target = <targetVar>.res`" + ` to declare its home.

See ` + "`formae://docs/forma-structure`" + ` for the full grammar and
` + "`formae://docs/stack-design`" + ` for multi-stack patterns.

## Project layout

A formae project is a directory with:

` + "```" + `
my-project/
  formae.conf.pkl      # CLI/agent configuration (plugin paths, etc.)
  PklProject           # PKL package dependencies (plugin schemas)
  main.pkl             # Your forma — entry point
  modules/             # Optional — reusable PKL modules
` + "```" + `

The PklProject file declares which plugin schemas you depend on.
` + "`formae project init`" + ` scaffolds all three.

## Cross-resource references

Resources reference each other by alias (` + "`.res`" + `) and dot-property access. The
schema dictates what's resolvable:

` + "```pkl" + `
local appBucket = new bucket.Bucket {
  label = "app-data"
  bucketName = "app-data-prod"
}

local handler = new function.Function {
  label = "api"
  environment = new Mapping<String, String|formae.Resolvable> {
    ["BUCKET"] = appBucket.res.bucketName
  }
}

forma {
  new formae.Stack { label = "default" }
  new formae.Target { label = "aws"; config = new aws.Config { region = "us-west-2" } }
  appBucket
  handler
}
` + "```" + `

` + "`local`" + ` keeps the binding out of the ` + "`forma {}`" + ` block while still allowing it
to be added explicitly. References between resources trigger dependency ordering
during apply.

## Labels and identification

Every resource has a ` + "`label`" + ` — a user-facing identifier within the stack.
The agent uses (stack, type, label) as a stable triplet, internally backed by
a KSUID. Labels must be unique within a (stack, type) pair.

## Going deeper

- Forma overview (canonical): https://docs.formae.io/en/latest/core-concepts/forma/
- Resource properties model: https://docs.formae.io/en/latest/core-concepts/properties/
- ` + "`.res`" + ` cross-resource refs: https://docs.formae.io/en/latest/core-concepts/res/
- Plugin-specific resource catalogs: https://hub.platform.engineering and per-plugin GitHub repos
- Schema annotations (what each field means): formae://docs/annotations
- PKL syntax fundamentals: formae://docs/pkl-primer
- Full forma grammar: formae://docs/forma-structure
- Multi-stack patterns: formae://docs/stack-design
`

const annotationsDoc = `# Formae Schema Annotations

Plugin authors define resource schemas in PKL using a set of annotations from
the formae module. End users rarely interact with these directly, but
understanding them helps when reading plugin source or troubleshooting
unexpected behavior.

## @formae.ResourceHint

Attached to a resource class. Declares the formae resource type identifier and
the field used as the resource's natural identifier.

` + "```pkl" + `
@formae.ResourceHint { type = "AWS::S3::Bucket"; identifier = "$.BucketName" }
open class Bucket extends Resource { ... }
` + "```" + `

- ` + "`type`" + ` is the on-the-wire type ID, format ` + "`NAMESPACE::Service::Resource`" + `.
- ` + "`identifier`" + ` is a JSONPath into the resource's serialized form, OR a literal field name for resources with synthetic IDs (e.g., ` + "`identifier = \"Label\"`" + `).

## @formae.FieldHint

Attached to a field on a resource. Defaults to mutable (createOnly = false).

` + "```pkl" + `
@formae.FieldHint { createOnly = true }
hidden bucketName: String

@formae.FieldHint { readOnly = true }
hidden arn: String|formae.Resolvable
` + "```" + `

- ` + "`createOnly = true`" + ` means changing the field forces resource replacement.
- ` + "`readOnly = true`" + ` means the value is server-assigned (output, not input).

## @formae.ConfigFieldHint

Attached to a field on a Target Config class. **Defaults to createOnly = true**
— the opposite of FieldHint. This is by design: Target fields typically identify
an immutable deployment environment (account ID, region, server URL), so
changing one implies replacing the Target and everything in it.

` + "```pkl" + `
@formae.ConfigFieldHint { createOnly = false }  // explicitly mutable
hidden host: String

@formae.ConfigFieldHint { createOnly = true }   // immutable (also the default)
hidden region: String
` + "```" + `

Always declare ` + "`createOnly`" + ` explicitly on ConfigFieldHint annotations to make
intent visible.

## @formae.Resolvable / hidden res

Resources expose their output ports via a ` + "`hidden res: ResolvableClass`" + ` field
that references a class containing the resolvable outputs. Other resources can
then write ` + "`<resource>.res.<field>`" + ` to bind to the output at apply time.

` + "```pkl" + `
class BucketResolvables {
  arn: formae.Resolvable
  domainName: formae.Resolvable
}

@formae.ResourceHint { type = "AWS::S3::Bucket"; identifier = "$.BucketName" }
open class Bucket extends Resource {
  hidden res: BucketResolvables
  // ...
}
` + "```" + `

## @formae.SubResourceHint

For nested resources where a parent owns sub-objects (e.g., S3 bucket lifecycle
rules). Sub-resources don't have their own lifecycle independent of the parent.

## Canonical prior-art examples

| Pattern | Where to see it |
|---|---|
| Resolvable computed outputs | formae-plugin-aws/schema/pkl/ses/emailidentity.pkl |
| Polymorphic Target auth (discriminated subclasses) | formae-plugin-k8s/schema/pkl/main/k8s.pkl |
| Cross-plugin Target Resolvables | formae-plugin-grafana/schema/pkl/grafana.pkl |
| Collection Resolvables (Mapping<String, formae.Resolvable>) | formae-plugin-compose |
| Synthetic identifier (identifier = "Label") | formae-plugin-atlas |

## Going deeper

- Schema reference (canonical PKL annotations): https://docs.formae.io/en/latest/plugin-sdk/reference/schema/
- Plugin SDK tutorial — schema: https://docs.formae.io/en/latest/plugin-sdk/tutorial/02-schema/
- Plugin SDK tutorial — target config: https://docs.formae.io/en/latest/plugin-sdk/tutorial/03-target/
- Plugin interface reference: https://docs.formae.io/en/latest/plugin-sdk/reference/plugin-interface/
- Plugin manifest format: https://docs.formae.io/en/latest/plugin-sdk/reference/manifest/
`

const troubleshootingDoc = `# Formae Troubleshooting

## "plugin not found" / "plugin <X> not installed on the agent"

The agent doesn't have the named plugin installed. Run
` + "`formae plugin install <name>`" + ` to add it from the Hub, or check the
agent's plugin directory (` + "`~/.pel/formae/plugins/`" + ` by default).

Concept: https://docs.formae.io/en/latest/core-concepts/plugin/

## "version conflict" during apply

Multiple resources in the changeset want incompatible versions of a shared
resource. Inspect the simulate output to see which resources contributed which
constraints.

## "drift detected" on apply

The actual cloud state diverged from formae's internal view since the last
sync. Options:
- Force-reconcile (overwrite cloud with formae's view): rerun apply with explicit confirmation
- Absorb drift (update IaC to match cloud): use the fix-code-drift workflow

Concept: https://docs.formae.io/en/latest/core-concepts/synchronization/

## "createOnly field changed" → replacement triggered

A field marked with ` + "`@formae.FieldHint { createOnly = true }`" + ` or
` + "`@formae.ConfigFieldHint { createOnly = true }`" + ` was modified. Formae
responds by destroying and recreating the resource. If unexpected, check
whether ConfigFieldHint's default (true) is biting — see formae://docs/annotations.

Schema reference: https://docs.formae.io/en/latest/plugin-sdk/reference/schema/

## "namespace mismatch" / resource type unknown

The forma file uses a resource type the agent's plugins don't recognize. Common
causes:
- Plugin not installed (see above)
- Resource type name mismatch — formae namespaces are uppercase
  (` + "`AWS::S3::Bucket`" + `, not ` + "`Aws::S3::Bucket`" + `)

## Apply mode confusion (reconcile vs patch)

Reconcile mode treats the forma as authoritative — anything in the stack not in
the forma will be destroyed. Patch mode only touches what's specified. If a
deploy unexpectedly removed resources, you likely used reconcile when patch was
intended (or vice versa).

Concept: https://docs.formae.io/en/latest/core-concepts/apply-modes/

## Commands stuck in "in_progress"

Long-running cloud operations (e.g., RDS modifications) can take many minutes.
Use ` + "`get_command_status`" + ` to inspect the latest activity. If the agent
itself crashed mid-operation, the command may need cancellation and retry.

## "verify-schema" reports 0 modules

PKL glob limitation: top-level resource files at ` + "`schema/pkl/<name>.pkl`" + `
weren't matched by older versions of ` + "`testutil/pkl/ImportsGenerator.pkl`" + `.
Fixed in formae main as of May 2026 — update to latest. Workaround: move
resources into a subdir like ` + "`schema/pkl/core/<name>.pkl`" + `.

## Discovery shows zero unmanaged resources

- Confirm the target's ` + "`discoverable = true`" + ` setting
- Check the agent's discovery rate-limiter (see get_agent_stats)
- Inspect the agent's discovery filters in the plugin config

Concept: https://docs.formae.io/en/latest/core-concepts/discovery/

## Going deeper

- Full docs site: https://docs.formae.io/en/latest/
- Core concepts index: https://docs.formae.io/en/latest/core-concepts/
- Per-plugin GitHub repos for plugin-specific issues
`
