package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/clientid"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
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

// contextResolver is the seam server tests substitute. The concrete resolver
// lives in execctx and its injection points are unexported there.
type contextResolver interface {
	Resolve(ctx context.Context, profileName string) (execctx.Context, error)
	Bin() string
}

// Server wraps the MCP server and the formae API client.
type Server struct {
	mcpServer      *mcp.Server
	hub            *HubClient
	forcedEndpoint string             // when set, empty-profile calls use this (tests / explicit)
	ctxResolver    contextResolver    // resolves per-call execution context
	clientID       *clientid.Resolver // resolves the Client-ID header value
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
		ctxResolver:    execctx.NewResolver(formaebin.NewBinResolver()),
		clientID:       clientid.NewResolver(),
	}

	s.registerTools()
	s.registerResources()
	s.registerPrompts()

	return s
}

// clientFor builds a FormaeClient for the given profile (empty = active/default).
func (s *Server) clientFor(ctx context.Context, profileName string) (*FormaeClient, error) {
	ec, err := s.resolveCtx(ctx, profileName)
	if err != nil {
		return nil, err
	}
	return newClientFromCtx(ec)
}

// resolveCtx returns the immutable execution context for an optional profile.
// Resolve once at the top of a handler; thread the result through eval and
// client construction so both steps agree on the binary and the connection.
//
// The profile name is validated here — that check is pure and cheap. Version
// gating is not: config.Resolve gates the CLI at 0.89.0, which already implies
// every earlier floor, and gating twice would make one call decide the binary
// twice.
//
// When a forcedEndpoint is configured and no profile is requested, it is used
// directly (this path is exercised by tests that inject a mock agent URL).
func (s *Server) resolveCtx(ctx context.Context, profileName string) (execctx.Context, error) {
	if profileName != "" {
		if err := profile.ValidateName(profileName); err != nil {
			return execctx.Context{}, err
		}
	}
	if profileName == "" && s.forcedEndpoint != "" {
		return execctx.Context{
			Conn:      config.Classic{URL: s.forcedEndpoint},
			FormaeBin: s.ctxResolver.Bin(),
		}, nil
	}
	return s.ctxResolver.Resolve(ctx, profileName)
}

// formaeBin returns the resolved formae binary path. Use this only in handlers
// that plan local file edits and never reach the agent; anything that resolves
// an execution context takes the binary from there.
func (s *Server) formaeBin() string {
	return s.ctxResolver.Bin()
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

func (s *Server) handleListResources(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListResourcesInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListResources(ctx, input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListStacks(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListStacks(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListTargets(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListTargetsInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListTargets(ctx, input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleGetCommandStatus(ctx context.Context, _ *mcp.CallToolRequest, input tools.GetCommandStatusInput) (*mcp.CallToolResult, any, error) {
	if input.CommandID == "" {
		return errorResult(fmt.Errorf("command_id is required")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := newClientFromCtx(ec)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.GetCommandStatus(ctx, input.CommandID, s.clientID.Resolve(ec.FormaeBin))
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListCommands(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListCommandsInput) (*mcp.CallToolResult, any, error) {
	maxResults := input.MaxResults
	if maxResults == "" {
		maxResults = "10"
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := newClientFromCtx(ec)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListCommands(ctx, input.Query, maxResults, s.clientID.Resolve(ec.FormaeBin))
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleGetAgentStats(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.GetAgentStats(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleCheckHealth(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := newClientFromCtx(ec)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := c.CheckHealth(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	msg := "Formae agent is healthy and reachable."
	if notice := s.buildSkewNotice(ctx, ec.FormaeBin, c); notice != "" {
		msg += "\n\n" + notice
	}
	return textResult(msg), nil, nil
}

// buildSkewNotice fetches the agent version and the local formae version and
// returns a skew notice when they differ, or "" when skew cannot be determined.
// It never returns an error — version-skew information is advisory only.
// The caller passes formaeBin (already resolved) to avoid a redundant resolveCtx call.
func (s *Server) buildSkewNotice(ctx context.Context, formaeBin string, c *FormaeClient) string {
	statsJSON, err := c.GetAgentStats(ctx)
	if err != nil {
		return ""
	}
	var stats struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(statsJSON, &stats); err != nil || stats.Version == "" {
		return ""
	}
	localVer, err := featuregate.DetectContext(ctx, formaeBin)
	if err != nil {
		return ""
	}
	return skewNotice(stats.Version, localVer)
}

func (s *Server) handleListPolicies(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ListPolicies(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListChangesSinceLastReconcile(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListChangesSinceLastReconcileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if input.Stack != "" {
		result, err := c.ListChangesSinceLastReconcile(ctx, input.Stack)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	}

	// No stack specified: fetch all stacks, then get drift for each
	stacksJSON, err := c.ListStacks(ctx)
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
		driftJSON, err := c.ListChangesSinceLastReconcile(ctx, stack.Label)
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

func (s *Server) handleExtractResources(ctx context.Context, _ *mcp.CallToolRequest, input tools.ExtractResourcesInput) (*mcp.CallToolResult, any, error) {
	if input.Query == "" {
		return errorResult(fmt.Errorf("query is required")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	tmpDir, err := os.MkdirTemp("", "formae-extract-*")
	if err != nil {
		return errorResult(fmt.Errorf("failed to create temp directory: %w", err)), nil, nil
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	outFile := tmpDir + "/extracted.pkl"
	args := []string{"extract", "--query", input.Query, "--yes"}
	if input.Profile != "" {
		args = append(args, "--profile", input.Profile)
	}
	args = append(args, outFile)
	cmd := exec.Command(ec.FormaeBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("formae extract failed: %w\noutput: %s", err, string(output))), nil, nil
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		return errorResult(fmt.Errorf("failed to read extracted file: %w", err)), nil, nil
	}

	notice := ""
	if c, cerr := newClientFromCtx(ec); cerr == nil {
		notice = s.buildSkewNotice(ctx, ec.FormaeBin, c)
	}
	return withNotice(textResult(string(content)), notice), nil, nil
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

func (s *Server) handleApplyForma(ctx context.Context, _ *mcp.CallToolRequest, input tools.ApplyFormaInput) (*mcp.CallToolResult, any, error) {
	if input.FilePath == "" {
		return errorResult(fmt.Errorf("file_path is required")), nil, nil
	}
	if input.Mode == "" {
		return errorResult(fmt.Errorf("mode is required (reconcile or patch)")), nil, nil
	}
	if input.Mode != "reconcile" && input.Mode != "patch" {
		return errorResult(fmt.Errorf("mode must be 'reconcile' or 'patch', got '%s'", input.Mode)), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	formaJSON, err := evalFormaFile(ec, input.FilePath)
	if err != nil {
		return errorResult(fmt.Errorf("failed to evaluate forma file: %w", err)), nil, nil
	}

	c, err := newClientFromCtx(ec)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.SubmitCommand(ctx, "apply", input.Mode, input.Simulate, input.Force, formaJSON, s.clientID.Resolve(ec.FormaeBin))
	if err != nil {
		return errorResult(err), nil, nil
	}
	return withNotice(jsonResult(result), s.buildSkewNotice(ctx, ec.FormaeBin, c)), nil, nil
}

func (s *Server) handleDestroyForma(ctx context.Context, _ *mcp.CallToolRequest, input tools.DestroyFormaInput) (*mcp.CallToolResult, any, error) {
	if input.FilePath == "" && input.Query == "" {
		return errorResult(fmt.Errorf("either file_path or query is required")), nil, nil
	}
	if input.FilePath != "" && input.Query != "" {
		return errorResult(fmt.Errorf("file_path and query are mutually exclusive")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := newClientFromCtx(ec)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if input.Query != "" {
		result, err := c.DestroyByQuery(ctx, input.Query, input.Simulate, s.clientID.Resolve(ec.FormaeBin))
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	}

	formaJSON, err := evalFormaFile(ec, input.FilePath)
	if err != nil {
		return errorResult(fmt.Errorf("failed to evaluate forma file: %w", err)), nil, nil
	}

	result, err := c.SubmitCommand(ctx, "destroy", "", input.Simulate, false, formaJSON, s.clientID.Resolve(ec.FormaeBin))
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleCancelCommands(ctx context.Context, _ *mcp.CallToolRequest, input tools.CancelCommandsInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := newClientFromCtx(ec)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.CancelCommands(ctx, input.Query, s.clientID.Resolve(ec.FormaeBin))
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleForceSync(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := c.ForceSync(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Resource synchronization triggered successfully."), nil, nil
}

func (s *Server) handleForceDiscover(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if err := c.ForceDiscover(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Resource discovery triggered successfully."), nil, nil
}

func (s *Server) handleForceCheckTTL(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	result, err := c.ForceCheckTTL(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleForceReconcileStack(ctx context.Context, _ *mcp.CallToolRequest, input tools.ForceReconcileStackInput) (*mcp.CallToolResult, any, error) {
	if input.Stack == "" {
		return errorResult(fmt.Errorf("stack is required")), nil, nil
	}
	c, err := s.clientFor(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	body, _, err := c.ForceReconcileStack(ctx, input.Stack)
	if err != nil {
		if body != nil {
			return errorResult(fmt.Errorf("%s: %s", err.Error(), string(body))), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(body), nil, nil
}

// Helpers

func evalFormaFile(ec execctx.Context, filePath string) ([]byte, error) {
	if strings.HasSuffix(filePath, ".json") {
		return os.ReadFile(filePath)
	}

	cmd := exec.Command(ec.FormaeBin, "eval", filePath, "--output-schema", "json", "--output-consumer", "machine")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("formae eval failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("formae eval failed: %w", err)
	}

	return output, nil
}

// withNotice appends notice as a separate text content block to res when notice
// is non-empty, leaving the primary content block pristine. When notice is ""
// it returns res unchanged. This keeps structured output (JSON receipts, PKL
// code) in the first block so callers can parse it without stripping a prefix.
func withNotice(res *mcp.CallToolResult, notice string) *mcp.CallToolResult {
	if notice == "" {
		return res
	}
	res.Content = append(res.Content, &mcp.TextContent{Text: notice})
	return res
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
