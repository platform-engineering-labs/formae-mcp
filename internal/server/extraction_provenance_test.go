// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
)

func TestActualExtractionIdentifiesPartialSource(t *testing.T) {
	const original = "extends \"@formae/forma.pkl\"\n// observed label: oob=drift\nforma {}\n"
	bin := filepath.Join(t.TempDir(), "formae")
	script := "#!/usr/bin/env python3\nimport sys,pathlib\nargs=sys.argv[1:]\nassert args[0]=='extract' and '--desired' not in args\npathlib.Path(args[-1]).write_text(" + fmt.Sprintf("%q", original) + ")\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	agent := mockAgent(t, map[string]http.HandlerFunc{"GET /api/v1/stats": func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{}`) }})
	defer agent.Close()
	session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, bin, filepath.Join(t.TempDir(), "registry.json"))
	for _, query := range []string{"stack:storage type:GCP::Storage::Bucket", "stack:storage"} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "extract_resources", Arguments: map[string]any{"query": query}})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("extract failed: %+v", result)
		}
		if got := result.Content[0].(*mcp.TextContent).Text; got != original {
			t.Fatalf("legacy Pkl source changed: %q", got)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var provenance struct {
			State           string `json:"state"`
			Partial         bool   `json:"partial"`
			Query           string `json:"query"`
			RecommendedTool string `json:"recommended_tool"`
		}
		if err := json.Unmarshal(encoded, &provenance); err != nil {
			t.Fatal(err)
		}
		if provenance.State != "actual" || !provenance.Partial || provenance.Query != query || provenance.RecommendedTool != "prepare_authoring" {
			t.Fatalf("actual extraction has no usable provenance: %s", encoded)
		}
		warning := ""
		for _, c := range result.Content[1:] {
			if text, ok := c.(*mcp.TextContent); ok {
				warning += text.Text
			}
		}
		for _, want := range []string{"ACTUAL", "PARTIAL", "unabsorbed", "prepare_authoring"} {
			if !strings.Contains(warning, want) {
				t.Errorf("missing warning %q: %s", want, warning)
			}
		}
	}
}

// This is a protocol integration test with a stub renderer and agent. It proves
// MCP requests complete desired state and preserves it through simulation; the
// agent fixture supplies the drift decision, not a live provider.
func TestDesiredAuthoringLeavesOOBLabelForDriftDecision(t *testing.T) {
	const desired = `{"Stacks":[{"Label":"storage"}],"Targets":[],"Resources":[{"Label":"bucket","Type":"GCP::Storage::Bucket","Stack":"storage","Properties":{"labels":{"environment":"dev"}}},{"Label":"companion","Type":"GCP::Storage::BucketIamMember","Stack":"storage","Properties":{"role":"reader"}}],"Extraction":{"CompleteStacks":[{"Label":"storage"}]}}`
	desiredReads, posts := 0, 0
	agent := mockAgent(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{"Capabilities":["desired-stack-extraction","shared-drift-resolution"]}`)
		},
		"GET /api/v1/plugins": func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, `{"plugins":[]}`) },
		"GET /api/v1/resources": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("state") != "desired" || r.URL.Query().Get("query") != `stack:"storage"` {
				t.Errorf("not complete desired extraction: %s", r.URL)
				http.Error(w, "wrong source", 400)
				return
			}
			desiredReads++
			_, _ = fmt.Fprint(w, desired)
		},
		"POST /api/v1/commands": func(w http.ResponseWriter, r *http.Request) {
			posts++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer func() {
				if r.MultipartForm != nil {
					_ = r.MultipartForm.RemoveAll()
				}
			}()
			if r.FormValue("mode") != "reconcile" || r.FormValue("simulate") != "true" || r.FormValue("force") == "true" || r.FormValue("resolution") != "" {
				t.Errorf("unexpected initial preview fields: %v", r.MultipartForm.Value)
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = f.Close() }()
			raw, err := io.ReadAll(f)
			if err != nil {
				t.Error(err)
				return
			}
			var forma struct {
				Resources []struct {
					Label      string
					Properties map[string]any
				}
			}
			if err := json.Unmarshal(raw, &forma); err != nil {
				t.Error(err)
				return
			}
			if len(forma.Resources) != 2 {
				t.Errorf("lost complete stack: %s", raw)
				return
			}
			labels := forma.Resources[0].Properties["labels"].(map[string]any)
			if labels["app"] != "demo" || labels["environment"] != "dev" {
				t.Errorf("lost intended change: %s", raw)
			}
			if _, exists := labels["oob"]; exists {
				t.Errorf("silently adopted OOB label: %s", raw)
			}
			w.WriteHeader(http.StatusConflict)
			_, _ = fmt.Fprint(w, `{"error":"ReconcileRejected","data":{"ObservationID":"observed-oob","ModifiedStacks":{"storage":{"ModifiedResources":[{"ResourceID":"bucket-id","Label":"bucket","ObservedCommand":"sync","Properties":{"labels":{"environment":"dev","oob":"drift"}}}]}}}}`)
		},
	})
	defer agent.Close()
	bin := filepath.Join(t.TempDir(), "formae")
	script := `#!/usr/bin/env python3
import sys,json,pathlib
args=sys.argv[1:]
if args[0]=='extract':
 assert args[1:3]==['--from-json','-']
 bundle=json.load(sys.stdin)
 path=pathlib.Path(args[-1]);path.write_text(json.dumps(bundle['Forma']))
 (path.parent/'PklProject').write_text('amends "pkl:Project"\n')
elif args[0]=='eval':
 print(pathlib.Path(args[1]).read_text())
else:
 sys.exit(1)
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	session := authoringSession(t, config.Classic{URL: agent.URL}, agent.URL, bin, filepath.Join(t.TempDir(), "registry.json"))
	var prepared authoringResult
	codebaseCall(t, session, "prepare_authoring", map[string]any{"stacks": []string{"storage"}, "temporary_directory": t.TempDir()}, &prepared)
	raw, err := os.ReadFile(prepared.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "oob") {
		t.Fatal("desired source contains unabsorbed label")
	}
	var forma map[string]any
	if err := json.Unmarshal(raw, &forma); err != nil {
		t.Fatal(err)
	}
	labels := forma["Resources"].([]any)[0].(map[string]any)["Properties"].(map[string]any)["labels"].(map[string]any)
	labels["app"] = "demo"
	raw, err = json.Marshal(forma)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prepared.FilePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "apply_forma", Arguments: map[string]any{"file_path": prepared.FilePath, "context": prepared.Context, "mode": "reconcile", "simulate": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(textContent(t, result), "observed-oob") || !strings.Contains(textContent(t, result), "ReconcileRejected") {
		t.Fatalf("drift decision not surfaced: %+v", result)
	}
	if desiredReads != 1 || posts != 1 {
		t.Fatalf("unexpected calls: desired=%d posts=%d", desiredReads, posts)
	}
}
