package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

type reportCaptureKey struct{}
type reportCapture struct {
	mu               sync.Mutex
	ec               execctx.Context
	resolved         bool
	reach            reach
	localDiagnostics string
}

// Fingerprints let supplementary user evidence be scrubbed against the original
// credential without retaining that credential for the event lifetime.
type reportSecretFingerprint struct {
	size   int
	digest [32]byte
}

func reportFingerprints(v secret.Value) []reportSecretFingerprint {
	if v.IsZero() {
		return nil
	}
	raw := v.Reveal()
	out := []reportSecretFingerprint{{len(raw), sha256.Sum256([]byte(raw))}}
	bare := strings.TrimPrefix(raw, "Bearer ")
	if bare != raw && bare != "" {
		out = append(out, reportSecretFingerprint{len(bare), sha256.Sum256([]byte(bare))})
	}
	return out
}
func scrubReportFingerprints(v string, fingerprints []reportSecretFingerprint) string {
	for _, f := range fingerprints {
		for i := 0; i+f.size <= len(v); {
			if sha256.Sum256([]byte(v[i:i+f.size])) == f.digest {
				v = v[:i] + secret.Mask + v[i+f.size:]
				i += len(secret.Mask)
			} else {
				i++
			}
		}
	}
	return v
}
func captureReportLocalDiagnostics(ctx context.Context, text string) {
	if c, ok := ctx.Value(reportCaptureKey{}).(*reportCapture); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.localDiagnostics = reportExcerpt(scrubReport(text, c.ec.Credential), 10000)
	}
}

type reportEvent struct {
	fingerprints []reportSecretFingerprint

	session                               *mcp.ServerSession
	created                               time.Time
	ec                                    execctx.Context
	tool, outcome, diagnostics, commandID string
}

func captureResolvedReportContext(ctx context.Context, ec execctx.Context) {
	if c, ok := ctx.Value(reportCaptureKey{}).(*reportCapture); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.resolved {
			c.ec = ec
			c.resolved = true
		}
	}
}
func captureReportReach(ctx context.Context, r reach) {
	if c, ok := ctx.Value(reportCaptureKey{}).(*reportCapture); ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		if r > c.reach {
			c.reach = r
		}
	}
}

// reportFailureFields traverses only status metadata and error evidence, never
// resource inventories or property values. A successful status query can still
// describe a failed infrastructure command.
func reportField(m map[string]any, name string) any {
	for k, v := range m {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}
func reportFailedState(v any) bool {
	s, _ := v.(string)
	switch strings.ToLower(s) {
	case "failed", "failure", "rejected", "canceled", "cancelled":
		return true
	}
	return false
}
func reportFailureFields(v any) (failed bool, commandID, evidence string) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	if commands, ok := reportField(m, "Commands").([]any); ok {
		count := 0
		ids := []string{}
		for _, c := range commands {
			f, id, e := reportFailureFields(c)
			if f {
				count++
				failed = true
				commandID = id
				evidence = e
				ids = append(ids, id)
			}
		}
		if count > 1 {
			commandID = ""
			evidence = "Multiple failed commands: " + strings.Join(ids, ", ") + ". Query each command separately for a report with its command_id."
		}
		return
	}
	for _, key := range []string{"State", "Status", "CommandState", "CommandStatus"} {
		if reportFailedState(reportField(m, key)) {
			failed = true
			evidence = "Command status: " + fmt.Sprint(reportField(m, key))
			break
		}
	}
	for _, key := range []string{"CommandId", "command_id"} {
		if id, ok := reportField(m, key).(string); ok {
			commandID = id
			break
		}
	}
	for _, key := range []string{"Error", "ErrorMessage", "failure_reason"} {
		if e, ok := reportField(m, key).(string); ok {
			evidence += "\n" + e
		}
	}
	if updates, ok := reportField(m, "ResourceUpdates").([]any); ok {
		for _, u := range updates {
			f, _, e := reportFailureFields(u)
			failed = failed || f
			if e != "" {
				evidence += "\n" + e
			}
		}
	}
	for _, key := range []string{"Command", "Result"} {
		if nested := reportField(m, key); nested != nil {
			f, id, e := reportFailureFields(nested)
			failed = failed || f
			if id != "" {
				commandID = id
			}
			if e != "" {
				evidence += "\n" + e
			}
		}
	}
	return
}
func reportPartialFailure(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if commands, ok := reportField(m, "Commands").([]any); ok {
		for _, c := range commands {
			if reportPartialFailure(c) {
				return true
			}
		}
	}
	failed, _, _ := reportFailureFields(v)
	if !failed {
		return false
	}
	if updates, ok := reportField(m, "ResourceUpdates").([]any); ok {
		for _, u := range updates {
			if m, ok := u.(map[string]any); ok {
				for _, k := range []string{"State", "Status"} {
					if state, ok := reportField(m, k).(string); ok && strings.EqualFold(state, "Success") {
						return true
					}
				}
			}
		}
	}
	return false
}
func (s *Server) captureReportEvent(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		req, ok := request.(*mcp.CallToolRequest)
		if !ok || method != "tools/call" || req.Params.Name == "prepare_bug_report" || req.Params.Name == "submit_bug_report" {
			return next(ctx, method, request)
		}
		capture := &reportCapture{}
		started := time.Now().UTC()
		result, err := next(context.WithValue(ctx, reportCaptureKey{}, capture), method, request)
		res, _ := result.(*mcp.CallToolResult)
		failed := err != nil || (res != nil && res.IsError)
		diagnostics := ""
		commandID := ""
		statusFailed := false
		partial := false
		if err != nil {
			diagnostics = err.Error()
		}
		if res != nil && len(res.Content) > 0 && (failed || req.Params.Name == "get_command_status") {
			if first, ok := res.Content[0].(*mcp.TextContent); ok {
				var value any
				if json.Unmarshal([]byte(first.Text), &value) == nil {
					// Successful tool results are evidence of failure only when a
					// status lookup describes exactly one command, never history pages.
					if !failed {
						m, ok := value.(map[string]any)
						if !ok {
							return result, err
						}
						commands, ok := reportField(m, "Commands").([]any)
						if !ok || len(commands) != 1 {
							return result, err
						}
					}
					f, id, e := reportFailureFields(value)
					statusFailed = f
					partial = reportPartialFailure(value)
					commandID = id
					diagnostics += e
				} else if failed {
					diagnostics += first.Text
				}
			}
		}
		if !failed && !statusFailed {
			return result, err
		}
		capture.mu.Lock()
		ec := capture.ec
		r := capture.reach
		local := capture.localDiagnostics
		capture.mu.Unlock()
		if _, ok := ec.Conn.(config.Hosted); !ok {
			return result, err
		}
		diagnostics = reportExcerpt(scrubReport(diagnostics, ec.Credential), 4000) + "\n" + local
		fingerprints := reportFingerprints(ec.Credential)
		commandID = reportExcerpt(scrubReport(commandID, ec.Credential), 256)
		ec.Credential = secret.Value{}
		outcome := "not_sent"
		if r == reachAttempted {
			outcome = "unknown"
		}
		if r == reachAnswered {
			outcome = "unknown"
		}
		if statusFailed {
			outcome = "failed"
		}
		if partial {
			outcome = "partial"
		}
		event := reportEvent{fingerprints: fingerprints, session: req.Session, created: started, ec: ec, tool: req.Params.Name, outcome: outcome, diagnostics: diagnostics, commandID: commandID}
		id := reportUUID()
		s.reportState.mu.Lock()
		s.cleanReports()
		if s.reportState.events == nil {
			s.reportState.events = make(map[string]reportEvent)
		}
		if len(s.reportState.events) >= 128 {
			var oldest string
			var when time.Time
			for k, e := range s.reportState.events {
				if oldest == "" || e.created.Before(when) {
					oldest = k
					when = e.created
				}
			}
			delete(s.reportState.events, oldest)
		}
		s.reportState.events[id] = event
		s.reportState.mu.Unlock()
		if res != nil {
			withNotice(res, fmt.Sprintf("Bug report event_id: %s", id))
		}
		return result, err
	}
}
