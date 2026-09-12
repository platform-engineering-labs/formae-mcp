package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/platform-engineering-labs/formae-mcp/internal/clientid"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
)

type workflowTelemetry struct {
	mu       sync.Mutex
	seen     map[string]bool
	client   *http.Client
	enabled  func(context.Context, execctx.Context) bool
	clientID func() (string, error)
}

func newWorkflowTelemetry() *workflowTelemetry {
	return &workflowTelemetry{seen: map[string]bool{}, client: &http.Client{Timeout: time.Second, CheckRedirect: refuseRedirects}, enabled: workflowUsageEnabled, clientID: clientid.NewResolver().Resolve}
}

// Reporting is best effort and never changes the outcome of a local preference
// or source selection. Each categorical combination is emitted once per process.
// Hosted distinct IDs represent installations, not individual people in a team.
// Classic IDs also include the existing local CLI client ID, so localhost on
// unrelated machines does not collapse into one installation.
func (w *workflowTelemetry) capture(ctx context.Context, ec execctx.Context, id codebase.Identity, mode string, pref codebase.DriftPreference, event string) {
	if w == nil {
		return
	}
	if event != "mcp_workflow_context" && event != "mcp_drift_preference_changed" {
		return
	}
	switch mode {
	case "none", "codebase", "selection_required", "unconfigured", "":
	default:
		return
	}
	if pref.Mode != "prompt" && pref.Mode != "auto_absorb_external" {
		return
	}
	identity, _ := json.Marshal(id)
	if id.Kind == "classic" {
		clientID, err := w.clientID()
		if err != nil {
			return
		}
		identity, _ = json.Marshal([]any{id, clientID})
	}
	hash := sha256.Sum256(identity)
	distinct := "mcp-installation-" + hex.EncodeToString(hash[:])
	properties := map[string]any{"$process_person_profile": false, "connection_mode": id.Kind, "drift_preference": pref.Mode, "drift_preference_explicit": pref.Explicit, "mcp_version": implementation().Version}
	if mode != "" {
		properties["codebase_mode"] = mode
	}
	// Same public ingestion key/destination as formae/internal/usage/posthog.go.
	payload, _ := json.Marshal(map[string]any{"api_key": "phc_4lcMdGyCWr8QKaPaWApjxAmrSv7hJidtVs7KP8e3Svx", "distinct_id": distinct, "event": event, "properties": properties})
	key := string(payload)
	w.mu.Lock()
	if w.seen[key] {
		w.mu.Unlock()
		return
	}
	if len(w.seen) >= 1024 {
		w.seen = map[string]bool{}
	}
	w.mu.Unlock()
	if !w.enabled(ctx, ec) {
		return
	}
	w.mu.Lock()
	if w.seen[key] {
		w.mu.Unlock()
		return
	}
	w.seen[key] = true
	w.mu.Unlock()
	// One bounded attempt per combination. Offline/blocked telemetry must not
	// delay every subsequent context call or retry an infrastructure operation.
	requestCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := workflowTelemetryRequest(requestCtx, payload)
	if err != nil {
		return
	}
	response, err := w.client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
}

// Read the existing non-secret CLI setting, using the resolved profile rather
// than the mutable active-profile pointer. Missing/old/unreadable settings mean
// no reporting. A changed connection is also refused.
func workflowUsageEnabled(ctx context.Context, ec execctx.Context) bool {
	if ec.FormaeBin == "" || ec.ProfileName == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ec.FormaeBin, "profile", "show", ec.ProfileName, "--output-consumer", "machine", "--output-schema", "json")
	var out workflowOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return false
	}
	var view struct {
		SchemaVersion int    `json:"schemaVersion"`
		Profile       string `json:"profile"`
		CLI           struct {
			Disabled   *bool `json:"disableUsageReporting"`
			Connection struct {
				Mode, URL, Endpoint, Installation string
				Port                              int
			} `json:"connection"`
		} `json:"cli"`
	}
	if json.Unmarshal(out.Bytes(), &view) != nil || view.SchemaVersion != 1 || view.Profile != ec.ProfileName || view.CLI.Disabled == nil || *view.CLI.Disabled {
		return false
	}
	var conn config.Connection
	c := view.CLI.Connection
	switch c.Mode {
	case "hosted":
		conn = config.Hosted{Endpoint: c.Endpoint, Installation: c.Installation}
	case "classic":
		conn = config.Classic{URL: c.URL, Port: c.Port}
	default:
		return false
	}
	got, err := codebase.IdentityForConnection(conn)
	expected, expectedErr := codebase.IdentityForConnection(ec.Conn)
	return err == nil && expectedErr == nil && got == expected
}

type workflowOutput struct{ bytes.Buffer }

func (b *workflowOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4<<20 {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(p)
}
