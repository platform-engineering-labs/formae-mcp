package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

func TestDriftGuidanceRefreshesPreferenceAndRequiresPinnedResolution(t *testing.T) {
	s := New("")
	r := codebase.Registry{Path: filepath.Join(t.TempDir(), "codebases.json")}
	s.codebaseRegistry = func() (codebase.Registry, error) { return r, nil }
	ec := execctx.Context{Conn: config.Classic{URL: "http://localhost", Port: 1234}}
	id, _ := codebase.IdentityForConnection(ec.Conn)
	for _, observation := range []string{"", `"ObservationID":"observation",`} {
		body := []byte(`{"error":"ReconcileRejected","data":{` + observation + `"ModifiedStacks":{"s":{"ModifiedResources":[{"ResourceID":"r","ExternalChangesOnly":true}]}}}}`)
		err := &commandHTTPError{status: 409, body: body}
		initial := s.applyErrorResult(context.Background(), ec, err)
		encoded, _ := json.Marshal(initial)
		if !strings.Contains(string(encoded), "keep") {
			t.Fatal("missing decision language")
		}
		if _, e := r.SetDriftPreference(context.Background(), id, "auto_absorb_external"); e != nil {
			t.Fatal(e)
		}
		got := s.applyErrorResult(context.Background(), ec, err)
		value := got.StructuredContent.(map[string]any)
		workflow := value["workflow"].(map[string]any)
		if workflow["drift_preference"].(codebase.DriftPreference).Mode != "auto_absorb_external" {
			t.Fatal("stale preference")
		}
		if workflow["resolution_available"] != (observation != "") {
			t.Fatalf("wrong resolution availability: %#v", workflow)
		}
	}
}
