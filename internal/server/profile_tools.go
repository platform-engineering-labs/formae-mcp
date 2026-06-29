package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/profile"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// profileMu serializes write_profile's check+rename against use_profile's
// active-pointer switch within this process.
var profileMu sync.Mutex

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

func (s *Server) handleUseProfile(_ context.Context, _ *mcp.CallToolRequest, input tools.UseProfileInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	if err := profile.ValidateName(input.Name); err != nil {
		return errorResult(err), nil, nil
	}
	profileMu.Lock()
	defer profileMu.Unlock()
	out, err := runFormaeProfile([]string{"use", input.Name})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(fmt.Sprintf("Switched active profile to %q.\n%s", input.Name, out)), nil, nil
}

func (s *Server) handleSaveProfile(_ context.Context, _ *mcp.CallToolRequest, input tools.SaveProfileInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	if err := profile.ValidateName(input.Name); err != nil {
		return errorResult(err), nil, nil
	}
	args := []string{"save", input.Name}
	if input.Force {
		args = append(args, "--force")
	}
	out, err := runFormaeProfile(args)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (s *Server) handleCreateProfile(_ context.Context, _ *mcp.CallToolRequest, input tools.CreateProfileInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	if err := profile.ValidateName(input.Name); err != nil {
		return errorResult(err), nil, nil
	}
	args := []string{"create", input.Name}
	if input.Force {
		args = append(args, "--force")
	}
	out, err := runFormaeProfile(args)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (s *Server) handleDeleteProfile(_ context.Context, _ *mcp.CallToolRequest, input tools.DeleteProfileInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	if err := profile.ValidateName(input.Name); err != nil {
		return errorResult(err), nil, nil
	}
	out, err := runFormaeProfile([]string{"delete", input.Name})
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (s *Server) handleDiffProfiles(_ context.Context, _ *mcp.CallToolRequest, input tools.DiffProfilesInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	if err := profile.ValidateName(input.A); err != nil {
		return errorResult(err), nil, nil
	}
	args := []string{"diff", input.A}
	if input.B != "" {
		if err := profile.ValidateName(input.B); err != nil {
			return errorResult(err), nil, nil
		}
		args = append(args, input.B)
	}
	// exit code 1 means "files differ", which is success for diff.
	out, err := runFormaeProfile(args, 1)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(out), nil, nil
}

func (s *Server) handleWriteProfile(_ context.Context, _ *mcp.CallToolRequest, input tools.WriteProfileInput) (*mcp.CallToolResult, any, error) {
	if err := featuregate.GuardFeature(featuregate.FeatureProfile); err != nil {
		return errorResult(err), nil, nil
	}
	path, err := profile.ProfilePath(input.Name)
	if err != nil {
		return errorResult(err), nil, nil
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return errorResult(fmt.Errorf("profile %q does not exist (use create_profile to create it): %w", input.Name, statErr)), nil, nil
	}
	profileMu.Lock()
	defer profileMu.Unlock()
	active, aerr := profile.ActiveProfile()
	if aerr != nil && !errors.Is(aerr, profile.ErrNotInitialized) {
		return errorResult(aerr), nil, nil
	}
	if aerr == nil && input.Name == active {
		return errorResult(fmt.Errorf("cannot rewrite the active profile %q — switch away with use_profile first, or write to a copy", input.Name)), nil, nil
	}
	if err := atomicWrite(path, []byte(input.Content)); err != nil {
		return errorResult(err), nil, nil
	}
	return textResult(fmt.Sprintf("Wrote profile %q.", input.Name)), nil, nil
}

// atomicWrite writes data to a temp file in the same dir and renames it over path.
// If the target already exists its permissions are preserved on the replacement.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-profile-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	// Preserve the existing file's permissions so a rewrite does not downgrade
	// them to the CreateTemp default (0600).
	if fi, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpName, fi.Mode().Perm())
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
