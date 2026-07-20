package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// stubStandaloneFixtureEval reports the standalone_fixture's contents to the
// resolvers without needing the formae binary: two stacks and one standalone
// policy, all in main.pkl.
func stubStandaloneFixtureEval(t *testing.T) {
	t.Helper()
	prev := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		if filepath.Base(path) == "main.pkl" {
			return []byte(`{"Stacks":[{"Label":"lifeline"},{"Label":"staging"}],` +
				`"Policies":[{"Label":"ephemeral-1h","Type":"ttl"}]}`), nil
		}
		return []byte(`{"Stacks":[],"Policies":[]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prev })
}

func TestCreateStandalonePolicyPlansInsertion(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_standalone_policy",
		Arguments: map[string]any{
			"label":            "nightly-drift",
			"policy_type":      "auto_reconcile",
			"interval_seconds": 300,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}

	var out struct {
		FilePath             string   `json:"file_path"`
		Operation            string   `json:"operation"`
		PKLSnippet           string   `json:"pkl_snippet"`
		InsertionAnchorStart int      `json:"insertion_anchor_start"`
		InsertionAnchorEnd   int      `json:"insertion_anchor_end"`
		ImportsToAdd         []string `json:"imports_to_add"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, textContent(t, result))
	}

	if out.Operation != "create" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "create")
	}
	if !strings.HasSuffix(out.FilePath, "main.pkl") {
		t.Errorf("got:\n%s\nwant:\na path ending in main.pkl", out.FilePath)
	}
	if !strings.Contains(out.PKLSnippet, "new formae.AutoReconcilePolicy {") {
		t.Errorf("got:\n%s\nwant:\nan AutoReconcilePolicy declaration", out.PKLSnippet)
	}
	if !strings.Contains(out.PKLSnippet, "interval = 5.min") {
		t.Errorf("got:\n%s\nwant:\na snippet carrying interval = 5.min", out.PKLSnippet)
	}
	// standalone_fixture/main.pkl closes its forma block on line 22.
	if out.InsertionAnchorStart != 22 || out.InsertionAnchorEnd != 22 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", out.InsertionAnchorStart, out.InsertionAnchorEnd, 22, 22)
	}
}

func TestCreateStandalonePolicyDuplicateLabelIsNoop(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_standalone_policy",
		Arguments: map[string]any{
			"label":       "ephemeral-1h",
			"policy_type": "ttl",
			"ttl_seconds": 7200,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}

	var out struct {
		Operation string   `json:"operation"`
		Notes     []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "noop")
	}
	if len(out.Notes) == 0 {
		t.Error("got:\nno notes\nwant:\na note explaining the duplicate label")
	}
}

func TestCreateStandalonePolicyMissingTTLSeconds(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_standalone_policy",
		Arguments: map[string]any{
			"label":       "broken",
			"policy_type": "ttl",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (ttl_seconds is required for policy_type=ttl)")
	}
}

// policiesHandler serves a fixed GET /api/v1/policies inventory.
func policiesHandler(t *testing.T, body string) map[string]http.HandlerFunc {
	t.Helper()
	return map[string]http.HandlerFunc{
		"GET /api/v1/policies": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, body)
		},
	}
}

const ttlPolicyInventory = `[{"Label":"ephemeral-1h","Type":"ttl",` +
	`"Config":{"Type":"ttl","Label":"ephemeral-1h","TTLSeconds":3600,"OnDependents":"abort"},` +
	`"AttachedStacks":["lifeline"]}]`

func TestAttachStandalonePolicyPlansInsertion(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t, ttlPolicyInventory))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "staging",
			"policy_label": "ephemeral-1h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}

	var out struct {
		FilePath             string `json:"file_path"`
		Operation            string `json:"operation"`
		PKLSnippet           string `json:"pkl_snippet"`
		InsertionAnchorStart int    `json:"insertion_anchor_start"`
		InsertionAnchorEnd   int    `json:"insertion_anchor_end"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "attach" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "attach")
	}
	if !strings.Contains(out.PKLSnippet, "policies = new Listing {") {
		t.Errorf("got:\n%s\nwant:\na wrapped listing (staging has no policies block)", out.PKLSnippet)
	}
	// standalone_fixture: the staging stack closes on line 21.
	if out.InsertionAnchorStart != 21 || out.InsertionAnchorEnd != 21 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", out.InsertionAnchorStart, out.InsertionAnchorEnd, 21, 21)
	}
}

func TestAttachStandalonePolicyAlreadyAttachedIsNoop(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t, ttlPolicyInventory))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "lifeline",
			"policy_label": "ephemeral-1h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	var out struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "noop")
	}
}

func TestAttachStandalonePolicyUnknownPolicy(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t, ttlPolicyInventory))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "staging",
			"policy_label": "does-not-exist",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (the policy is unknown to the agent)")
	}
	if !strings.Contains(textContent(t, result), "ephemeral-1h") {
		t.Errorf("got:\n%s\nwant:\nan error listing the known policy labels", textContent(t, result))
	}
}

func TestAttachStandalonePolicyInlineConflict(t *testing.T) {
	withFixtureWorkspace(t, "standalone_conflict_fixture")
	prev := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		return []byte(`{"Stacks":[{"Label":"lifeline"}],"Policies":[]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prev })

	agent := mockAgent(t, policiesHandler(t, ttlPolicyInventory))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "lifeline",
			"policy_label": "ephemeral-1h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (the stack has an inline TTL policy)")
	}
	if !strings.Contains(textContent(t, result), "inline") {
		t.Errorf("got:\n%s\nwant:\nan error mentioning the inline policy", textContent(t, result))
	}
}

func TestAttachStandalonePolicySameTypeStandaloneConflict(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	// A second TTL standalone already attached to lifeline.
	agent := mockAgent(t, policiesHandler(t,
		`[{"Label":"ephemeral-1h","Type":"ttl","Config":{"Type":"ttl","TTLSeconds":3600,"OnDependents":"abort"},"AttachedStacks":["lifeline"]},`+
			`{"Label":"ephemeral-2h","Type":"ttl","Config":{"Type":"ttl","TTLSeconds":7200,"OnDependents":"abort"},"AttachedStacks":[]}]`))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "lifeline",
			"policy_label": "ephemeral-2h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (lifeline already has a TTL standalone attached)")
	}
	if !strings.Contains(textContent(t, result), "ephemeral-1h") {
		t.Errorf("got:\n%s\nwant:\nan error naming the conflicting standalone", textContent(t, result))
	}
}

func TestDetachStandalonePolicyPlansRemoval(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "detach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "lifeline",
			"policy_label": "ephemeral-1h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}

	var out struct {
		FilePath                  string   `json:"file_path"`
		Operation                 string   `json:"operation"`
		SourceAnchorStart         int      `json:"source_anchor_start"`
		SourceAnchorEnd           int      `json:"source_anchor_end"`
		ExistingResolvableSnippet string   `json:"existing_resolvable_snippet"`
		Notes                     []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "detach" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "detach")
	}
	// The resolvable is lifeline's only policy, so the whole wrapper goes:
	// standalone_fixture lines 13-15.
	if out.SourceAnchorStart != 13 || out.SourceAnchorEnd != 15 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", out.SourceAnchorStart, out.SourceAnchorEnd, 13, 15)
	}
	if out.ExistingResolvableSnippet == "" {
		t.Error("got:\nempty existing_resolvable_snippet\nwant:\nthe removed text")
	}
}

func TestDetachStandalonePolicyNotAttachedIsNoop(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "detach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "staging",
			"policy_label": "ephemeral-1h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}
	var out struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "noop")
	}
}

func TestDeleteStandalonePolicyRefusesWhileAttached(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t, ttlPolicyInventory)) // AttachedStacks: ["lifeline"]
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "delete_standalone_policy",
		Arguments: map[string]any{"label": "ephemeral-1h"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (the policy is still attached to lifeline)")
	}
	if !strings.Contains(textContent(t, result), "lifeline") {
		t.Errorf("got:\n%s\nwant:\nan error listing the attached stacks", textContent(t, result))
	}
}

func TestDeleteStandalonePolicyPlansDeletion(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t,
		`[{"Label":"ephemeral-1h","Type":"ttl","Config":{"Type":"ttl","Label":"ephemeral-1h","TTLSeconds":3600,"OnDependents":"abort"},"AttachedStacks":[]}]`))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "delete_standalone_policy",
		Arguments: map[string]any{"label": "ephemeral-1h"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(t, result))
	}

	var out struct {
		FilePath              string `json:"file_path"`
		Operation             string `json:"operation"`
		SourceAnchorStart     int    `json:"source_anchor_start"`
		SourceAnchorEnd       int    `json:"source_anchor_end"`
		ExistingPolicySnippet string `json:"existing_policy_snippet"`
		DestroyFormaPKL       string `json:"destroy_forma_pkl"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "delete" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "delete")
	}
	// standalone_fixture declares the standalone on lines 5-9.
	if out.SourceAnchorStart != 5 || out.SourceAnchorEnd != 9 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", out.SourceAnchorStart, out.SourceAnchorEnd, 5, 9)
	}
	// The destroy forma must be self-contained and carry the agent's config.
	if !strings.Contains(out.DestroyFormaPKL, `amends "@formae/forma.pkl"`) {
		t.Errorf("got:\n%s\nwant:\na self-contained forma with an amends clause", out.DestroyFormaPKL)
	}
	if !strings.Contains(out.DestroyFormaPKL, "ttl = 1.h") {
		t.Errorf("got:\n%s\nwant:\nthe agent's stored TTL (3600s -> 1.h)", out.DestroyFormaPKL)
	}
}

func TestDeleteStandalonePolicyUnknownPolicy(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t, ttlPolicyInventory))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "delete_standalone_policy",
		Arguments: map[string]any{"label": "does-not-exist"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (the policy is unknown to the agent)")
	}
}

// TestAttachStandalonePolicyDeclaredButNotYetApplied is the regression test for
// the create-then-attach workflow: the skill declares a standalone, edits the
// file, and attaches BEFORE the first apply. At that moment the policy exists in
// source but not in the agent inventory. Attaching must still work.
func TestAttachStandalonePolicyDeclaredButNotYetApplied(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	// The agent knows NO policies yet — nothing has been applied.
	agent := mockAgent(t, policiesHandler(t, `[]`))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "staging",
			"policy_label": "ephemeral-1h",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("got:\nerror: %s\nwant:\nsuccess (the policy is declared in source, just not applied yet)",
			textContent(t, result))
	}

	var out struct {
		Operation  string   `json:"operation"`
		PKLSnippet string   `json:"pkl_snippet"`
		Notes      []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "attach" {
		t.Errorf("got:\n%s\nwant:\n%s", out.Operation, "attach")
	}
	found := false
	for _, n := range out.Notes {
		if strings.Contains(n, "not yet applied") {
			found = true
		}
	}
	if !found {
		t.Errorf("got:\n%v\nwant:\na note saying the policy is declared but not yet applied", out.Notes)
	}
}

// TestAttachStandalonePolicyUnknownEverywhere pins the genuine not-found case:
// absent from BOTH the agent and the workspace source.
func TestAttachStandalonePolicyUnknownEverywhere(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")
	stubStandaloneFixtureEval(t)

	agent := mockAgent(t, policiesHandler(t, `[]`))
	defer agent.Close()

	session := connectTestServer(t, agent.URL)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "attach_standalone_policy",
		Arguments: map[string]any{
			"stack":        "staging",
			"policy_label": "never-declared",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("got:\nsuccess\nwant:\nerror (the policy exists neither in the agent nor in source)")
	}
}

// TestCreateStandalonePolicyDuplicateInAnotherFileIsNoop is the regression test
// for the multi-file duplicate case: the label is already declared in a
// DIFFERENT file than the one create would target. Emitting a second
// declaration would produce two standalone policies sharing a label.
func TestCreateStandalonePolicyDuplicateInAnotherFileIsNoop(t *testing.T) {
	withFixtureWorkspace(t, "standalone_fixture")

	// main.pkl has the stacks (so it wins "main forma file"), but the policy is
	// declared in a separate other.pkl.
	prev := injectedEvalForTest
	injectedEvalForTest = func(path string) ([]byte, error) {
		if filepath.Base(path) == "main.pkl" {
			return []byte(`{"Stacks":[{"Label":"lifeline"},{"Label":"staging"}],"Policies":[]}`), nil
		}
		return []byte(`{"Stacks":[],"Policies":[{"Label":"shared-ttl","Type":"ttl"}]}`), nil
	}
	t.Cleanup(func() { injectedEvalForTest = prev })

	session := connectTestServer(t, "http://localhost:1")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "create_standalone_policy",
		Arguments: map[string]any{
			"label":       "shared-ttl",
			"policy_type": "ttl",
			"ttl_seconds": 3600,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success (noop), got error: %s", textContent(t, result))
	}

	var out struct {
		Operation string   `json:"operation"`
		Notes     []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(textContent(t, result)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s (the label is already declared in another file)", out.Operation, "noop")
	}
	if len(out.Notes) == 0 {
		t.Error("got:\nno notes\nwant:\na note naming the file that already declares it")
	}
}
