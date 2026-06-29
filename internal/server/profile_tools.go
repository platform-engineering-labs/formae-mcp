package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/profile"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// runFormaeProfile shells out to `formae profile <args...>` and returns combined
// output. okExit lists non-zero exit codes that are NOT errors (e.g. diff's 1).
func runFormaeProfile(args []string, okExit ...int) (string, error) {
	cmd := exec.Command("formae", append([]string{"profile"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			for _, code := range okExit {
				if exitErr.ExitCode() == code {
					return string(out), nil
				}
			}
		}
		return string(out), fmt.Errorf("formae profile %v failed: %w\noutput: %s", args, err, string(out))
	}
	return string(out), nil
}

func (s *Server) handleListProfiles(_ context.Context, _ *mcp.CallToolRequest, _ tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	out, err := runFormaeProfile([]string{"list", "--output-consumer", "machine", "--output-schema", "json"})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (s *Server) handleCurrentProfile(_ context.Context, _ *mcp.CallToolRequest, _ tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	out, err := runFormaeProfile([]string{"current", "--output-consumer", "machine", "--output-schema", "json"})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (s *Server) handleReadProfile(_ context.Context, _ *mcp.CallToolRequest, input tools.ReadProfileInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	path, err := profile.ProfilePath(input.Name)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errorResult(fmt.Errorf("profile %q not found: %w", input.Name, err)), nil, nil
	}
	return textResult(string(data)), nil, nil
}
