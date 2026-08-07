package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSkewNotice(t *testing.T) {
	cases := []struct {
		agent, formae, wantSub string
	}{
		{"0.90.0", "0.90.0", ""},
		{"", "0.90.0", ""},
		{"0.92.0", "0.88.0", "agent is newer"},
		{"0.88.0", "0.92.0", "formae is newer"},
	}
	for _, c := range cases {
		got := skewNotice(c.agent, c.formae)
		if c.wantSub == "" {
			if got != "" {
				t.Errorf("skewNotice(%s,%s) = %q, want empty", c.agent, c.formae, got)
			}
			continue
		}
		if !strings.Contains(got, c.wantSub) {
			t.Errorf("skewNotice(%s,%s) = %q, want substring %q", c.agent, c.formae, got, c.wantSub)
		}
	}
}

// TestWithNoticePreservesFirstBlock verifies that withNotice appends the notice
// as a second content block and leaves the first block untouched — so a JSON
// receipt from apply_forma stays valid JSON and PKL from extract_resources stays
// intact regardless of version skew.
func TestWithNoticePreservesFirstBlock(t *testing.T) {
	const jsonPayload = `{"commandId":"abc123","status":"queued"}`
	const pklPayload = `amends "package://platform.engineering/aws@0.1.0#/S3Bucket.pkl"`
	const notice = "Version skew: the connected agent is newer (0.92.0) than your local formae (0.88.0)."

	t.Run("apply JSON receipt", func(t *testing.T) {
		res := withNotice(jsonResult(json.RawMessage(jsonPayload)), notice)
		if len(res.Content) != 2 {
			t.Fatalf("expected 2 content blocks, got %d", len(res.Content))
		}
		first := res.Content[0].(*mcp.TextContent).Text
		if err := json.Unmarshal([]byte(first), new(map[string]any)); err != nil {
			t.Errorf("first block is not valid JSON: %v (got: %s)", err, first)
		}
		second := res.Content[1].(*mcp.TextContent).Text
		if !strings.Contains(second, "skew") {
			t.Errorf("second block should contain the notice, got: %s", second)
		}
	})

	t.Run("extract PKL output", func(t *testing.T) {
		res := withNotice(textResult(pklPayload), notice)
		if len(res.Content) != 2 {
			t.Fatalf("expected 2 content blocks, got %d", len(res.Content))
		}
		first := res.Content[0].(*mcp.TextContent).Text
		if first != pklPayload {
			t.Errorf("first block modified: got %q, want %q", first, pklPayload)
		}
		second := res.Content[1].(*mcp.TextContent).Text
		if !strings.Contains(second, "skew") {
			t.Errorf("second block should contain the notice, got: %s", second)
		}
	})

	t.Run("no notice leaves single block", func(t *testing.T) {
		res := withNotice(jsonResult(json.RawMessage(jsonPayload)), "")
		if len(res.Content) != 1 {
			t.Fatalf("expected 1 content block when no notice, got %d", len(res.Content))
		}
	})
}
