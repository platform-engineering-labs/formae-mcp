// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

const authoringMetadataName = ".formae-authoring.json"

type authoringMetadata struct {
	Version   int               `json:"version"`
	Directory string            `json:"directory"`
	Identity  codebase.Identity `json:"identity"`
	Stacks    []string          `json:"stacks"`
}

type authoringResult struct {
	FilePath       string              `json:"file_path"`
	ProjectPath    string              `json:"project_path"`
	Context        tools.SourceContext `json:"context"`
	CompleteStacks []string            `json:"complete_stacks"`
	SchemaPlugins  []schemaPlugin      `json:"schema_plugins"`
	Instructions   string              `json:"instructions"`
	Warnings       string              `json:"warnings,omitempty"`
	Diagnostics    json.RawMessage     `json:"diagnostics,omitempty"`
}

type schemaPlugin struct {
	Type             string `json:"type"`
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	InstalledVersion string `json:"installedVersion"`
}

func (c *FormaeClient) renderingPlugins(ctx context.Context) (json.RawMessage, []schemaPlugin, error) {
	body, status, err := c.get(ctx, "/api/v1/plugins", url.Values{"scope": {"installed"}}, retryOnce)
	if err != nil {
		return nil, nil, err
	}
	if err := c.unroutedIf(status); err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, fmt.Errorf("plugin metadata unavailable: HTTP %d: %s", status, body)
	}
	var doc struct{ Plugins json.RawMessage }
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, nil, err
	}
	var plugins []schemaPlugin
	if len(doc.Plugins) == 0 || bytes.Equal(doc.Plugins, []byte("null")) {
		return nil, nil, fmt.Errorf("agent omitted installed plugin metadata")
	}
	if err := json.Unmarshal(doc.Plugins, &plugins); err != nil {
		return nil, nil, err
	}
	// Render remote dependencies from the exact installed namespace/version. Local
	// paths in this metadata belong to the agent, never to the caller's machine.
	return doc.Plugins, plugins, nil
}

func (c *FormaeClient) desiredStacks(ctx context.Context, stacks []string) (json.RawMessage, error) {
	selectors := make([]string, len(stacks))
	// Query phrases escape quotes and backslashes, but do not decode Go's
	// Unicode escapes. Preserve every other character in the validated label.
	escape := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	for i, stack := range stacks {
		selectors[i] = `stack:"` + escape.Replace(stack) + `"`
	}
	body, status, err := c.get(ctx, "/api/v1/resources", url.Values{"state": {"desired"}, "query": {strings.Join(selectors, " ")}}, retryOnce)
	if err != nil {
		return nil, err
	}
	if err := c.unroutedIf(status); err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("desired extraction failed: HTTP %d: %s", status, body)
	}
	var forma struct {
		Extraction *struct{ CompleteStacks []struct{ Label string } }
	}
	if err := json.Unmarshal(body, &forma); err != nil {
		return nil, err
	}
	if forma.Extraction == nil {
		return nil, fmt.Errorf("agent returned no complete desired stack scope")
	}
	got := []string{}
	for _, stack := range forma.Extraction.CompleteStacks {
		got = append(got, stack.Label)
	}
	slices.Sort(got)
	want := slices.Clone(stacks)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return nil, fmt.Errorf("agent returned a different complete desired stack scope")
	}
	return body, nil
}

func authoringStackLabels(existing, added []string) ([]string, error) {
	labels := append(slices.Clone(existing), added...)
	seen := map[string]bool{}
	for _, label := range labels {
		if strings.TrimSpace(label) == "" || label == "unmanaged" || seen[label] {
			return nil, fmt.Errorf("stack labels must be unique nonempty managed stack literals")
		}
		for _, r := range label {
			if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
				return nil, fmt.Errorf("stack label contains unsupported control characters")
			}
		}
		seen[label] = true
	}
	slices.Sort(labels)
	return labels, nil
}

func canonicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("temporary_directory must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if canonical != filepath.Clean(path) {
		return "", fmt.Errorf("temporary directory must be canonical and cannot cross a symlink")
	}
	stat, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("temporary directory is not a directory")
	}
	return canonical, nil
}

func (s *Server) prepareAuthoring(ctx context.Context, ec execctx.Context, c *FormaeClient, input tools.PrepareAuthoringInput) (*authoringResult, error) {
	labels, err := authoringStackLabels(input.Stacks, input.NewStacks)
	if err != nil {
		return nil, err
	}
	directory, err := canonicalDirectory(input.TemporaryDirectory)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("temporary_directory must be empty; retain previous source for review or retry and choose a fresh disposable directory")
	}
	identity, err := codebase.IdentityForConnection(ec.Conn)
	if err != nil {
		return nil, err
	}
	registry, err := s.codebaseRegistry()
	if err != nil {
		return nil, err
	}
	if _, err := registry.Select(ctx, identity, codebase.Request{Mode: codebase.ModeNone}); err != nil {
		return nil, err
	}
	if err := c.requireCapability(ctx, "desired-stack-extraction"); err != nil {
		return nil, fmt.Errorf("%w; this agent requires an existing complete codebase for authoring", err)
	}
	if err := c.requireCapability(ctx, "shared-drift-resolution"); err != nil {
		return nil, err
	}
	forma := json.RawMessage(`{"Stacks":[],"Resources":[],"Targets":[],"Extraction":{"CompleteStacks":[]}}`)
	if len(input.Stacks) > 0 {
		forma, err = c.desiredStacks(ctx, input.Stacks)
		if err != nil {
			return nil, err
		}
	}
	if len(input.NewStacks) > 0 || len(input.Targets) > 0 {
		forma, err = augmentAuthoring(ctx, c, forma, input)
		if err != nil {
			return nil, err
		}
	}
	// Keep the agent's read-side repair diagnostics alongside the generated
	// source. They do not authorize dropping or rebinding any declaration.
	var extracted struct {
		Extraction struct{ Diagnostics json.RawMessage }
	}
	if err := json.Unmarshal(forma, &extracted); err != nil {
		return nil, err
	}
	plugins, summary, err := c.renderingPlugins(ctx)
	if err != nil {
		return nil, err
	}
	bundle, err := json.Marshal(struct {
		Forma   json.RawMessage
		Plugins json.RawMessage
	}{forma, plugins})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}
	// An exclusive local marker claims an empty caller-owned directory before
	// rendering. Partial failures remain inspectable and are never reused.
	metadata := authoringMetadata{Version: 0, Directory: directory, Identity: identity, Stacks: labels}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	marker, err := os.OpenFile(filepath.Join(directory, authoringMetadataName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := marker.Write(raw)
	closeErr := marker.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	path := filepath.Join(directory, "main.pkl")
	cmd := commandWithContext(ctx, ec.FormaeBin, "extract", "--from-json", "-", "--yes", path)
	cmd.Dir = directory
	cmd.Stdin = bytes.NewReader(bundle)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("offline Pkl rendering failed; inspect and remove the disposable directory when no retry needs it: %w\n%s", err, safeSubprocessOutput(output, ec.Credential))
	}
	for _, file := range []string{path, filepath.Join(directory, "PklProject")} {
		if _, err := containedRegularFile(directory, file); err != nil {
			return nil, err
		}
		if err := os.Chmod(file, 0600); err != nil {
			return nil, err
		}
	}
	// Publish readiness only after the renderer completed and both full source
	// and dependency project were verified. Failed partial output cannot apply.
	metadata.Version = 1
	raw, err = json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	ready, err := os.CreateTemp(directory, ".formae-authoring-ready-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(ready.Name()) }()
	_, writeErr = ready.Write(raw)
	closeErr = ready.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := os.Rename(ready.Name(), filepath.Join(directory, authoringMetadataName)); err != nil {
		return nil, err
	}
	return &authoringResult{FilePath: path, ProjectPath: filepath.Join(directory, "PklProject"), Context: tools.SourceContext{Mode: codebase.ModeNone, TemporaryDirectory: directory}, CompleteStacks: labels, SchemaPlugins: summary, Diagnostics: extracted.Extraction.Diagnostics, Warnings: safeSubprocessOutput(output, ec.Credential), Instructions: "This is mode none: follow the server instructions and discuss infrastructure outcomes and user choices; keep temporary paths, source files and dependency work internal. With no complete stacks selected, this workspace supports stackless declarations such as initial targets only; prepare a fresh workspace with explicit stack scope before adding stacks, resources or generators. Read the diagnostics and complete main.pkl. Unresolved desired references are repair placeholders: explain the failure and use the user's intended change to remove the owning declaration, rewire its reference, or explicitly restore its dependency before evaluation. Ask when that choice is unclear; never silently drop declarations or bind to a same-name replacement. A complete reconcile can withdraw failed-create intent with zero cloud operations; that does not confirm cloud absence. Read and edit the complete main.pkl; preserve all existing stack declarations, policies, targets, references and generators. Add schema dependencies for new resource namespaces using the reported installed versions, then resolve PklProject dependencies. Simulate soft reconcile with this exact context; resolve all actionable drift and confirm the final combined plan before submitting. Keep the directory and exact submission until outcome/retry inspection is complete. After terminal outcome, or abandoning a preview with no real submission, the harness removes this disposable directory. It is never a maintained project or automatically registered."}, nil
}

func augmentAuthoring(ctx context.Context, c *FormaeClient, raw json.RawMessage, input tools.PrepareAuthoringInput) (json.RawMessage, error) {
	var forma map[string]json.RawMessage
	if err := json.Unmarshal(raw, &forma); err != nil {
		return nil, err
	}
	if len(input.NewStacks) > 0 {
		listed, err := c.ListStacks(ctx)
		if err != nil {
			return nil, err
		}
		var existing []struct{ Label string }
		if err := json.Unmarshal(listed, &existing); err != nil {
			return nil, err
		}
		for _, stack := range existing {
			if slices.Contains(input.NewStacks, stack.Label) {
				return nil, fmt.Errorf("stack %q already exists; extract its complete desired declaration", stack.Label)
			}
		}
		var stacks []json.RawMessage
		if err := json.Unmarshal(forma["Stacks"], &stacks); err != nil {
			return nil, err
		}
		var extraction map[string]json.RawMessage
		if err := json.Unmarshal(forma["Extraction"], &extraction); err != nil {
			return nil, err
		}
		var complete []json.RawMessage
		if err := json.Unmarshal(extraction["CompleteStacks"], &complete); err != nil {
			return nil, err
		}
		for _, label := range input.NewStacks {
			entry, _ := json.Marshal(struct{ Label string }{label})
			stacks = append(stacks, entry)
			complete = append(complete, entry)
		}
		forma["Stacks"], _ = json.Marshal(stacks)
		extraction["CompleteStacks"], _ = json.Marshal(complete)
		forma["Extraction"], _ = json.Marshal(extraction)
	}
	if len(input.Targets) > 0 {
		listed, err := c.ListTargets(ctx, "")
		if err != nil {
			return nil, err
		}
		var available []json.RawMessage
		if err := json.Unmarshal(listed, &available); err != nil {
			return nil, err
		}
		targets := []json.RawMessage{}
		if len(forma["Targets"]) > 0 {
			if err := json.Unmarshal(forma["Targets"], &targets); err != nil {
				return nil, err
			}
		}
		seen := map[string]bool{}
		for _, raw := range targets {
			var target struct{ Label string }
			if err := json.Unmarshal(raw, &target); err != nil {
				return nil, err
			}
			seen[target.Label] = true
		}
		for _, label := range input.Targets {
			if seen[label] {
				continue
			}
			found := false
			for _, raw := range available {
				var target struct{ Label string }
				if err := json.Unmarshal(raw, &target); err != nil {
					return nil, err
				}
				if target.Label == label {
					targets = append(targets, raw)
					seen[label] = true
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("target %q is not configured on the connected installation", label)
			}
		}
		forma["Targets"], _ = json.Marshal(targets)
	}
	return json.Marshal(forma)
}

func containedRegularFile(directory, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("source file must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(directory, canonical)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("source file is outside its disposable directory")
	}
	if canonical != filepath.Clean(path) {
		return "", fmt.Errorf("disposable source cannot cross a symlink")
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("source is not a regular file")
	}
	return canonical, nil
}

func validateDisposable(identity codebase.Identity, source *tools.SourceContext, path string, stacks []string) error {
	if source.BindingID != "" {
		return fmt.Errorf("no-codebase mode cannot name a project binding")
	}
	directory, err := canonicalDirectory(source.TemporaryDirectory)
	if err != nil {
		return err
	}
	marker, err := containedRegularFile(directory, filepath.Join(directory, authoringMetadataName))
	if err != nil {
		return err
	}
	file, err := os.Open(marker)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return err
	}
	if len(raw) > 64<<10 {
		return fmt.Errorf("invalid disposable context metadata")
	}
	var metadata authoringMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return fmt.Errorf("invalid disposable context metadata")
	}
	if metadata.Version != 1 || metadata.Directory != directory {
		return fmt.Errorf("disposable preparation is incomplete, the directory was moved, or context metadata is invalid")
	}
	if metadata.Identity != identity {
		return fmt.Errorf("disposable source belongs to another installation")
	}
	for _, stack := range stacks {
		if !slices.Contains(metadata.Stacks, stack) {
			return fmt.Errorf("stack %q is outside the prepared complete-stack scope; prepare a fresh workspace", stack)
		}
	}
	_, err = containedRegularFile(directory, path)
	return err
}

func (s *Server) handlePrepareAuthoring(ctx context.Context, _ *mcp.CallToolRequest, input tools.PrepareAuthoringInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := s.prepareAuthoring(ctx, ec, c, input)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(raw)), nil, nil
}
