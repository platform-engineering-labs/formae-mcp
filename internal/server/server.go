package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/profile"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
	"github.com/platform-engineering-labs/formae-mcp/internal/version"
)

const serverName = "formae-mcp"

// implementation describes this server in the MCP handshake. The version is
// read from internal/version so the handshake and the CLI --version flag share
// a single, build-injectable source of truth.
func implementation() *mcp.Implementation {
	return &mcp.Implementation{
		Name:    serverName,
		Version: version.String(),
	}
}

// Server wraps the MCP server and the formae API client.
type Server struct {
	mcpServer      *mcp.Server
	hub            *HubClient
	forcedEndpoint string // when set, empty-profile calls use this (tests / explicit)
}

// New creates a new formae MCP server connected to the given agent endpoint.
func New(endpoint string) *Server {
	mcpServer := mcp.NewServer(
		implementation(),
		&mcp.ServerOptions{
			Instructions: serverInstructions,
		},
	)

	s := &Server{
		mcpServer:      mcpServer,
		hub:            NewHubClient(),
		forcedEndpoint: endpoint,
	}

	s.registerTools()
	s.registerResources()
	s.registerPrompts()

	return s
}

// clientFor builds a FormaeClient for the given profile (empty = active/default).
// A non-empty profile is version-gated and name-validated; endpoint resolution
// hard-errors for an unresolvable requested/active profile.
func (s *Server) clientFor(profileName string) (*FormaeClient, error) {
	if profileName != "" {
		if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
			return nil, err
		}
		if err := profile.ValidateName(profileName); err != nil {
			return nil, err
		}
	} else if s.forcedEndpoint != "" {
		return NewFormaeClient(s.forcedEndpoint), nil
	}
	url, port, err := config.AgentEndpoint(profileName)
	if err != nil {
		return nil, err
	}
	return NewFormaeClient(url + ":" + port), nil
}

// Run starts the MCP server with the given transport.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	return s.mcpServer.Run(ctx, transport)
}

func (s *Server) registerTools() {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	// Read-only tools
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_resources",
		Description: tools.ListResourcesDescription,
		Annotations: readOnly,
	}, s.handleListResources)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_stacks",
		Description: tools.ListStacksDescription,
		Annotations: readOnly,
	}, s.handleListStacks)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_targets",
		Description: tools.ListTargetsDescription,
		Annotations: readOnly,
	}, s.handleListTargets)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_command_status",
		Description: tools.GetCommandStatusDescription,
		Annotations: readOnly,
	}, s.handleGetCommandStatus)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_commands",
		Description: tools.ListCommandsDescription,
		Annotations: readOnly,
	}, s.handleListCommands)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_agent_stats",
		Description: tools.GetAgentStatsDescription,
		Annotations: readOnly,
	}, s.handleGetAgentStats)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "check_health",
		Description: tools.CheckHealthDescription,
		Annotations: readOnly,
	}, s.handleCheckHealth)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_policies",
		Description: tools.ListPoliciesDescription,
		Annotations: readOnly,
	}, s.handleListPolicies)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_changes_since_last_reconcile",
		Description: tools.ListChangesSinceLastReconcileDescription,
		Annotations: readOnly,
	}, s.handleListChangesSinceLastReconcile)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "extract_resources",
		Description: tools.ExtractResourcesDescription,
		Annotations: readOnly,
	}, s.handleExtractResources)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "list_profiles", Description: tools.ListProfilesDescription, Annotations: readOnly,
	}, s.handleListProfiles)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "current_profile", Description: tools.CurrentProfileDescription, Annotations: readOnly,
	}, s.handleCurrentProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "read_profile", Description: tools.ReadProfileDescription, Annotations: readOnly,
	}, s.handleReadProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "use_profile", Description: tools.UseProfileDescription, Annotations: &mcp.ToolAnnotations{}}, s.handleUseProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "save_profile", Description: tools.SaveProfileDescription, Annotations: &mcp.ToolAnnotations{}}, s.handleSaveProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "create_profile", Description: tools.CreateProfileDescription, Annotations: &mcp.ToolAnnotations{}}, s.handleCreateProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "delete_profile", Description: tools.DeleteProfileDescription, Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)}}, s.handleDeleteProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "diff_profiles", Description: tools.DiffProfilesDescription, Annotations: readOnly}, s.handleDiffProfiles)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "write_profile", Description: tools.WriteProfileDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleWriteProfile)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_hub_plugins",
		Description: tools.SearchHubPluginsDescription,
		Annotations: readOnly,
	}, s.handleSearchHubPlugins)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_hub_plugin",
		Description: tools.GetHubPluginDescription,
		Annotations: readOnly,
	}, s.handleGetHubPlugin)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_plugin_examples",
		Description: tools.ListPluginExamplesDescription,
		Annotations: readOnly,
	}, s.handleListPluginExamples)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_plugin_example",
		Description: tools.GetPluginExampleDescription,
		Annotations: readOnly,
	}, s.handleGetPluginExample)

	// Mutation tools
	destructive := boolPtr(true)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "apply_forma",
		Description: tools.ApplyFormaDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive},
	}, s.handleApplyForma)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "destroy_forma",
		Description: tools.DestroyFormaDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive},
	}, s.handleDestroyForma)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "cancel_commands",
		Description: tools.CancelCommandsDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleCancelCommands)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_sync",
		Description: tools.ForceSyncDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleForceSync)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_discover",
		Description: tools.ForceDiscoverDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleForceDiscover)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_check_ttl",
		Description: tools.ForceCheckTTLDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: destructive},
	}, s.handleForceCheckTTL)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_reconcile_stack",
		Description: tools.ForceReconcileStackDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleForceReconcileStack)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_inline_policy",
		Description: tools.CreateInlinePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleCreateInlinePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_standalone_policy",
		Description: tools.CreateStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleCreateStandalonePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "attach_standalone_policy",
		Description: tools.AttachStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleAttachStandalonePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "detach_standalone_policy",
		Description: tools.DetachStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleDetachStandalonePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_standalone_policy",
		Description: tools.DeleteStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleDeleteStandalonePolicy)
}

// Tool handlers — read-only

func (s *Server) handleListResources(_ context.Context, _ *mcp.CallToolRequest, input tools.ListResourcesInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListResources(input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListStacks(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListStacks()
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListTargets(_ context.Context, _ *mcp.CallToolRequest, input tools.ListTargetsInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListTargets(input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleGetCommandStatus(_ context.Context, _ *mcp.CallToolRequest, input tools.GetCommandStatusInput) (*mcp.CallToolResult, any, error) {
	if input.CommandID == "" {
		return errorResult(fmt.Errorf("command_id is required")), nil, nil
	}
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.GetCommandStatus(input.CommandID, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListCommands(_ context.Context, _ *mcp.CallToolRequest, input tools.ListCommandsInput) (*mcp.CallToolResult, any, error) {
	maxResults := input.MaxResults
	if maxResults == "" {
		maxResults = "10"
	}
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListCommands(input.Query, maxResults, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleGetAgentStats(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.GetAgentStats()
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleCheckHealth(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := c.CheckHealth(); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Formae agent is healthy and reachable."), nil, nil
}

func (s *Server) handleListPolicies(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListPolicies()
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListChangesSinceLastReconcile(_ context.Context, _ *mcp.CallToolRequest, input tools.ListChangesSinceLastReconcileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if input.Stack != "" {
		result, err := c.ListChangesSinceLastReconcile(input.Stack)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	}

	// No stack specified: fetch all stacks, then get drift for each
	stacksJSON, err := c.ListStacks()
	if err != nil {
		return errorResult(fmt.Errorf("failed to list stacks: %w", err)), nil, nil
	}

	var stacks []struct {
		Label string `json:"Label"`
	}
	if err := json.Unmarshal(stacksJSON, &stacks); err != nil {
		return errorResult(fmt.Errorf("failed to parse stacks: %w", err)), nil, nil
	}

	type stackDrift struct {
		Stack             string          `json:"Stack"`
		ModifiedResources json.RawMessage `json:"ModifiedResources"`
	}
	var results []stackDrift

	for _, stack := range stacks {
		driftJSON, err := c.ListChangesSinceLastReconcile(stack.Label)
		if err != nil {
			return errorResult(fmt.Errorf("failed to get drift for stack %s: %w", stack.Label, err)), nil, nil
		}

		// Parse to check if there are modifications
		var drift struct {
			ModifiedResources json.RawMessage `json:"ModifiedResources"`
		}
		if err := json.Unmarshal(driftJSON, &drift); err != nil {
			return errorResult(fmt.Errorf("failed to parse drift for stack %s: %w", stack.Label, err)), nil, nil
		}

		results = append(results, stackDrift{
			Stack:             stack.Label,
			ModifiedResources: drift.ModifiedResources,
		})
	}

	aggregated, err := json.Marshal(results)
	if err != nil {
		return errorResult(fmt.Errorf("failed to marshal results: %w", err)), nil, nil
	}
	return jsonResult(aggregated), nil, nil
}

func (s *Server) handleExtractResources(_ context.Context, _ *mcp.CallToolRequest, input tools.ExtractResourcesInput) (*mcp.CallToolResult, any, error) {
	if input.Query == "" {
		return errorResult(fmt.Errorf("query is required")), nil, nil
	}
	if input.Profile != "" {
		if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
			return errorResult(err), nil, nil
		}
		if err := profile.ValidateName(input.Profile); err != nil {
			return errorResult(err), nil, nil
		}
	}

	tmpDir, err := os.MkdirTemp("", "formae-extract-*")
	if err != nil {
		return errorResult(fmt.Errorf("failed to create temp directory: %w", err)), nil, nil
	}
	defer os.RemoveAll(tmpDir)

	outFile := tmpDir + "/extracted.pkl"
	args := []string{"extract", "--query", input.Query, "--yes"}
	if input.Profile != "" {
		args = append(args, "--profile", input.Profile)
	}
	args = append(args, outFile)
	cmd := exec.Command("formae", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("formae extract failed: %w\noutput: %s", err, string(output))), nil, nil
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		return errorResult(fmt.Errorf("failed to read extracted file: %w", err)), nil, nil
	}

	return textResult(string(content)), nil, nil
}

func (s *Server) handleSearchHubPlugins(_ context.Context, _ *mcp.CallToolRequest, input tools.SearchHubPluginsInput) (*mcp.CallToolResult, any, error) {
	plugins, err := s.hub.SearchPlugins(input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(plugins)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

func (s *Server) handleGetHubPlugin(_ context.Context, _ *mcp.CallToolRequest, input tools.GetHubPluginInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		return errorResult(fmt.Errorf("name is required")), nil, nil
	}
	d, err := s.hub.GetPlugin(input.Name)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(d)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

func (s *Server) handleListPluginExamples(_ context.Context, _ *mcp.CallToolRequest, input tools.ListPluginExamplesInput) (*mcp.CallToolResult, any, error) {
	if input.Plugin == "" {
		return errorResult(fmt.Errorf("plugin is required")), nil, nil
	}
	result, err := s.hub.ListExamples(input.Plugin, input.Version)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

func (s *Server) handleGetPluginExample(_ context.Context, _ *mcp.CallToolRequest, input tools.GetPluginExampleInput) (*mcp.CallToolResult, any, error) {
	if input.Plugin == "" {
		return errorResult(fmt.Errorf("plugin is required")), nil, nil
	}
	if input.Example == "" {
		return errorResult(fmt.Errorf("example is required")), nil, nil
	}
	result, err := s.hub.GetExample(input.Plugin, input.Example, input.Version)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

// Tool handlers — mutations

func (s *Server) handleApplyForma(_ context.Context, _ *mcp.CallToolRequest, input tools.ApplyFormaInput) (*mcp.CallToolResult, any, error) {
	if input.FilePath == "" {
		return errorResult(fmt.Errorf("file_path is required")), nil, nil
	}
	if input.Mode == "" {
		return errorResult(fmt.Errorf("mode is required (reconcile or patch)")), nil, nil
	}
	if input.Mode != "reconcile" && input.Mode != "patch" {
		return errorResult(fmt.Errorf("mode must be 'reconcile' or 'patch', got '%s'", input.Mode)), nil, nil
	}
	if input.Profile != "" {
		if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
			return errorResult(err), nil, nil
		}
		if err := profile.ValidateName(input.Profile); err != nil {
			return errorResult(err), nil, nil
		}
	}

	formaJSON, err := evalFormaFile(input.FilePath)
	if err != nil {
		return errorResult(fmt.Errorf("failed to evaluate forma file: %w", err)), nil, nil
	}

	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.SubmitCommand("apply", input.Mode, input.Simulate, input.Force, formaJSON, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleDestroyForma(_ context.Context, _ *mcp.CallToolRequest, input tools.DestroyFormaInput) (*mcp.CallToolResult, any, error) {
	if input.FilePath == "" && input.Query == "" {
		return errorResult(fmt.Errorf("either file_path or query is required")), nil, nil
	}
	if input.FilePath != "" && input.Query != "" {
		return errorResult(fmt.Errorf("file_path and query are mutually exclusive")), nil, nil
	}
	if input.Profile != "" {
		if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
			return errorResult(err), nil, nil
		}
		if err := profile.ValidateName(input.Profile); err != nil {
			return errorResult(err), nil, nil
		}
	}

	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if input.Query != "" {
		result, err := c.DestroyByQuery(input.Query, input.Simulate, "formae-mcp")
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	}

	formaJSON, err := evalFormaFile(input.FilePath)
	if err != nil {
		return errorResult(fmt.Errorf("failed to evaluate forma file: %w", err)), nil, nil
	}

	result, err := c.SubmitCommand("destroy", "", input.Simulate, false, formaJSON, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleCancelCommands(_ context.Context, _ *mcp.CallToolRequest, input tools.CancelCommandsInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.CancelCommands(input.Query, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleForceSync(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := c.ForceSync(); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Resource synchronization triggered successfully."), nil, nil
}

func (s *Server) handleForceDiscover(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := c.ForceDiscover(); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Resource discovery triggered successfully."), nil, nil
}

func (s *Server) handleForceCheckTTL(_ context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ForceCheckTTL()
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleForceReconcileStack(_ context.Context, _ *mcp.CallToolRequest, input tools.ForceReconcileStackInput) (*mcp.CallToolResult, any, error) {
	if input.Stack == "" {
		return errorResult(fmt.Errorf("stack is required")), nil, nil
	}
	c, err := s.clientFor(input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	body, _, err := c.ForceReconcileStack(input.Stack)
	if err != nil {
		if body != nil {
			return errorResult(fmt.Errorf("%s: %s", err.Error(), string(body))), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(body), nil, nil
}

// Helpers

func evalFormaFile(filePath string) ([]byte, error) {
	if strings.HasSuffix(filePath, ".json") {
		return os.ReadFile(filePath)
	}

	cmd := exec.Command("formae", "eval", filePath, "--output-schema", "json", "--output-consumer", "machine")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("formae eval failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("formae eval failed: %w", err)
	}

	return output, nil
}

func jsonResult(data json.RawMessage) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Error: %s", err.Error())},
		},
		IsError: true,
	}
}
