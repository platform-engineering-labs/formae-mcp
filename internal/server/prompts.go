package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) registerPrompts() {
	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "check_drift",
		Description: "Check for infrastructure drift and help resolve it. Shows sync drift (out-of-band changes) and patch drift (unreconciled patches).",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Check for infrastructure drift",
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: `Check my infrastructure for drift. First use list_resources to get an overview of managed resources, then use get_agent_stats to see resource counts.

Show me:
1. Any out-of-band changes detected by the agent's continuous synchronization
2. Any patches that haven't been reconciled yet

For each piece of drift found, help me decide whether to:
- Overwrite: undo the change by force-reconciling
- Absorb: incorporate the change into my IaC codebase
- Extract to file: save the current state as PKL for manual review

Group drift by stack and process one stack at a time.`},
				},
			},
		}, nil
	})

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "discover_resources",
		Description: "Find unmanaged resources in cloud accounts that aren't managed by formae yet.",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Discover unmanaged resources",
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: `Find all unmanaged resources in my cloud infrastructure. Use list_resources with query 'managed:false' to see what the formae agent has discovered.

Present the results grouped by resource type, showing:
- Resource type and label
- Key properties
- Which target/account they belong to

Help me decide which resources to bring under management.`},
				},
			},
		}, nil
	})

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "import_resources",
		Description: "Bring one or more unmanaged resources under formae management.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "query",
				Description: "Optional query to filter which unmanaged resources to import (e.g., 'type:AWS::S3::Bucket')",
				Required:    false,
			},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		query := "managed:false"
		if q, ok := req.Params.Arguments["query"]; ok && q != "" {
			query = "managed:false " + q
		}
		return &mcp.GetPromptResult{
			Description: "Import unmanaged resources",
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: "I want to bring unmanaged resources under formae management. Query the resources with: " + query + `

For each resource or group of resources I select:
1. Extract the resource as PKL infrastructure code
2. Either:
   a. Help me incorporate it into my existing IaC codebase following its patterns and conventions
   b. Or save it as a standalone forma file

If incorporating into an existing codebase:
- Assign to an appropriate stack
- Follow existing naming conventions
- Verify by running apply --mode reconcile --simulate
- Loop until the simulation shows only the expected "bring under management" changes`},
				},
			},
		}, nil
	})

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "deploy_infrastructure",
		Description: "Apply a forma file to deploy or update infrastructure with simulation and confirmation.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "file_path",
				Description: "Path to the forma file to apply",
				Required:    true,
			},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		filePath := req.Params.Arguments["file_path"]
		return &mcp.GetPromptResult{
			Description: "Deploy infrastructure",
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: "I want to deploy infrastructure using the forma file at: " + filePath + `

Please follow this workflow:
1. First, simulate the apply (--simulate) to preview changes
2. Show me what will be created, updated, and destroyed
3. Ask for my confirmation before proceeding
4. If I confirm, apply the changes
5. Monitor the command until completion and report results`},
				},
			},
		}, nil
	})

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "patch_infrastructure",
		Description: "Make an urgent targeted change to infrastructure without a full reconcile. Use for incident response.",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Patch infrastructure",
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: `I need to make an urgent targeted change to my infrastructure.

Help me:
1. Identify the resource(s) to modify
2. Create a minimal forma file with only the targeted change
3. Simulate the patch to verify it does what I want
4. Apply the patch with my confirmation
5. Monitor until complete

IMPORTANT: After the patch is applied, remind me that this change will appear as drift until I reconcile my IaC code. Suggest running the drift check workflow when the incident is resolved.`},
				},
			},
		}, nil
	})

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "build_plugin",
		Description: "Build a new formae resource plugin from scratch using the plugin SDK tutorial.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "provider",
				Description: "The cloud provider or technology to build a plugin for (e.g., 'cloudflare', 'datadog')",
				Required:    true,
			},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		provider := req.Params.Arguments["provider"]
		return &mcp.GetPromptResult{
			Description: "Build a new formae plugin for " + provider,
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: "I want to build a new formae resource plugin for " + provider + `.

Follow the plugin SDK tutorial at https://docs.formae.io/en/latest/plugin-sdk/tutorial/ from start to finish.

Steps:
1. Research the provider's API to understand resources and their properties
2. Scaffold the plugin using formae plugin init --no-input
3. Follow the tutorial's TDD workflow
4. Run conformance tests (make conformance-test) to verify
5. Create a working example
6. Update the README

Reference existing plugins (aws, azure, gcp, oci, ovh) as prior art.

CRITICAL: After ANY code changes, run 'make install' before running conformance tests. The tests run against the installed plugin binary, not source code.`},
				},
			},
		}, nil
	})

	s.mcpServer.AddPrompt(&mcp.Prompt{
		Name:        "add_resource_type",
		Description: "Add support for a new resource type to an existing formae plugin.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "resource_type",
				Description: "The resource type to add support for (e.g., 'DNS::Record', 'Compute::Instance')",
				Required:    true,
			},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		resourceType := req.Params.Arguments["resource_type"]
		return &mcp.GetPromptResult{
			Description: "Add resource type: " + resourceType,
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{Text: "I want to add support for the " + resourceType + ` resource type to my existing formae plugin.

Steps:
1. Research the provider API for this resource type
2. Define the PKL schema for the resource
3. Implement the CRUD operations in the plugin
4. Add unit tests following TDD
5. Run conformance tests to verify
6. Add an example and update documentation

Follow the existing patterns in the plugin for consistency. Always run 'make install' before running tests.`},
				},
			},
		}, nil
	})
}
