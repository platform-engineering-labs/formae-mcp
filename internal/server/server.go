package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

const (
	serverName    = "formae-mcp"
	serverVersion = "0.1.0"
)

// Server wraps the MCP server and the formae API client.
type Server struct {
	mcpServer *mcp.Server
	client    *FormaeClient
}

// New creates a new formae MCP server connected to the given agent endpoint.
func New(endpoint string) *Server {
	client := NewFormaeClient(endpoint)

	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		&mcp.ServerOptions{
			Instructions: serverInstructions,
		},
	)

	s := &Server{
		mcpServer: mcpServer,
		client:    client,
	}

	s.registerTools()
	s.registerResources()
	s.registerPrompts()

	return s
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
		Name:        "list_plugins",
		Description: tools.ListPluginsDescription,
		Annotations: readOnly,
	}, s.handleListPlugins)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_drift",
		Description: tools.ListDriftDescription,
		Annotations: readOnly,
	}, s.handleListDrift)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "extract_resources",
		Description: tools.ExtractResourcesDescription,
		Annotations: readOnly,
	}, s.handleExtractResources)

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
}

// Tool handlers — read-only

func (s *Server) handleListResources(_ context.Context, _ *mcp.CallToolRequest, input tools.ListResourcesInput) (*mcp.CallToolResult, any, error) {
	result, err := s.client.ListResources(input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListStacks(_ context.Context, _ *mcp.CallToolRequest, input tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	result, err := s.client.ListStacks()
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleListTargets(_ context.Context, _ *mcp.CallToolRequest, input tools.ListTargetsInput) (*mcp.CallToolResult, any, error) {
	result, err := s.client.ListTargets(input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleGetCommandStatus(_ context.Context, _ *mcp.CallToolRequest, input tools.GetCommandStatusInput) (*mcp.CallToolResult, any, error) {
	if input.CommandID == "" {
		return errorResult(fmt.Errorf("command_id is required")), nil, nil
	}
	result, err := s.client.GetCommandStatus(input.CommandID, "formae-mcp")
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
	result, err := s.client.ListCommands(input.Query, maxResults, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleGetAgentStats(_ context.Context, _ *mcp.CallToolRequest, input tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	result, err := s.client.GetAgentStats()
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleCheckHealth(_ context.Context, _ *mcp.CallToolRequest, input tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	if err := s.client.CheckHealth(); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Formae agent is healthy and reachable."), nil, nil
}

func (s *Server) handleListPlugins(_ context.Context, _ *mcp.CallToolRequest, input tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	cmd := exec.Command("formae", "plugin", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errorResult(fmt.Errorf("failed to list plugins: %w\noutput: %s", err, string(output))), nil, nil
	}
	return textResult(string(output)), nil, nil
}

func (s *Server) handleListDrift(_ context.Context, _ *mcp.CallToolRequest, input tools.ListDriftInput) (*mcp.CallToolResult, any, error) {
	if input.Stack != "" {
		result, err := s.client.ListDrift(input.Stack)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	}

	// No stack specified: fetch all stacks, then get drift for each
	stacksJSON, err := s.client.ListStacks()
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
		driftJSON, err := s.client.ListDrift(stack.Label)
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

	tmpDir, err := os.MkdirTemp("", "formae-extract-*")
	if err != nil {
		return errorResult(fmt.Errorf("failed to create temp directory: %w", err)), nil, nil
	}
	defer os.RemoveAll(tmpDir)

	outFile := tmpDir + "/extracted.pkl"
	cmd := exec.Command("formae", "extract", "--query", input.Query, "--yes", outFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("formae extract failed: %w\noutput: %s", err, string(output))), nil, nil
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		return errorResult(fmt.Errorf("failed to read extracted file: %w", err)), nil, nil
	}

	return textResult(string(content)), nil, nil
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

	formaJSON, err := evalFormaFile(input.FilePath)
	if err != nil {
		return errorResult(fmt.Errorf("failed to evaluate forma file: %w", err)), nil, nil
	}

	result, err := s.client.SubmitCommand("apply", input.Mode, input.Simulate, input.Force, formaJSON, "formae-mcp")
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

	if input.Query != "" {
		result, err := s.client.DestroyByQuery(input.Query, input.Simulate, "formae-mcp")
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result), nil, nil
	}

	formaJSON, err := evalFormaFile(input.FilePath)
	if err != nil {
		return errorResult(fmt.Errorf("failed to evaluate forma file: %w", err)), nil, nil
	}

	result, err := s.client.SubmitCommand("destroy", "", input.Simulate, false, formaJSON, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleCancelCommands(_ context.Context, _ *mcp.CallToolRequest, input tools.CancelCommandsInput) (*mcp.CallToolResult, any, error) {
	result, err := s.client.CancelCommands(input.Query, "formae-mcp")
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result), nil, nil
}

func (s *Server) handleForceSync(_ context.Context, _ *mcp.CallToolRequest, input tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	if err := s.client.ForceSync(); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Resource synchronization triggered successfully."), nil, nil
}

func (s *Server) handleForceDiscover(_ context.Context, _ *mcp.CallToolRequest, input tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	if err := s.client.ForceDiscover(); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult("Resource discovery triggered successfully."), nil, nil
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
