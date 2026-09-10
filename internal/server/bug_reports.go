package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
	"github.com/platform-engineering-labs/formae-mcp/internal/version"
)

const supportRecipient = "support@formae.ai"
const classicReportHelp = "Hosted reporting requires a hosted profile. For classic installations use https://github.com/platform-engineering-labs/formae/issues or https://discord.gg/hr6dHaW76k."

// Stores are bounded across all sessions and expire after an hour. No disk persistence.
const reportTTL = time.Hour
const maxStoredReports = 64

type prepareBugReportInput struct {
	OccurredAt string            `json:"occurred_at,omitempty" jsonschema:"Original failure time as RFC3339 for an assistant supplied report; omitted means preparation time with explicit provenance."`
	Outcome    string            `json:"outcome,omitempty" jsonschema:"For assistant supplied reports: not_sent, unknown, partial, failed, or not_applicable. Never guess whether infrastructure mutations took effect."`
	Versions   map[string]string `json:"versions,omitempty" jsonschema:"Known cli, agent or plugin versions only. Missing versions are explicitly noted. MCP version is supplied by this server."`

	EventID      string `json:"event_id,omitempty" jsonschema:"Reference from the original tool call. Freezes that call's installation; do not substitute a current profile."`
	Profile      string `json:"profile,omitempty" jsonschema:"Explicit hosted profile only when no captured event exists. This fallback is recorded as assistant supplied provenance."`
	Tool         string `json:"tool,omitempty" jsonschema:"Required when using an explicit profile fallback; name the original tool or CLI operation. Captured events supply this automatically."`
	Component    string `json:"component" jsonschema:"formae, plugin, mcp, or unknown"`
	Summary      string `json:"summary"`
	Expected     string `json:"expected"`
	Actual       string `json:"actual"`
	Evidence     string `json:"evidence"`
	Diagnostics  string `json:"diagnostics,omitempty" jsonschema:"Only relevant local CLI diagnostics, at most 16384 UTF-8 bytes. No agent logs, inventories, full forma files or transcripts."`
	CommandID    string `json:"command_id,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	Plugin       string `json:"plugin,omitempty"`
}
type submitBugReportInput struct {
	ReportID  string `json:"report_id"`
	Confirmed bool   `json:"confirmed" jsonschema:"True only when the user authorized sending this preview to support@formae.ai."`
}
type bugReport struct {
	SchemaVersion int               `json:"schema_version"`
	ReportID      string            `json:"report_id"`
	OccurredAt    string            `json:"occurred_at"`
	Tool          string            `json:"tool"`
	Component     string            `json:"component"`
	Summary       string            `json:"summary"`
	Expected      string            `json:"expected"`
	Actual        string            `json:"actual"`
	Evidence      string            `json:"evidence"`
	Outcome       string            `json:"outcome"`
	CommandID     string            `json:"command_id,omitempty"`
	ResourceType  string            `json:"resource_type,omitempty"`
	Plugin        string            `json:"plugin,omitempty"`
	ProfileName   string            `json:"profile_name,omitempty"`
	Versions      map[string]string `json:"versions"`
	Diagnostics   string            `json:"diagnostics,omitempty"`
}
type bugReceipt struct {
	ReportID  string `json:"report_id"`
	Status    string `json:"status"`
	Recipient string `json:"recipient"`
	Duplicate bool   `json:"duplicate"`
}
type storedReport struct {
	mu      sync.Mutex
	session *mcp.ServerSession
	created time.Time
	ec      execctx.Context
	body    []byte
	receipt *bugReceipt
}
type reportState struct {
	mu      sync.Mutex
	reports map[string]*storedReport
	events  map[string]reportEvent
}

func reportSession(req *mcp.CallToolRequest) *mcp.ServerSession {
	if req == nil {
		return nil
	}
	return req.Session
}
func reportUUID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var reportSecrets = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`(?im)(?:authorization|proxy-authorization|cookie|set-cookie)\s*[:=][^\r\n]*`),
	regexp.MustCompile(`(?i)(?:bearer|basic)\s+[A-Za-z0-9+/_.=~-]+`),
	regexp.MustCompile(`(?i)["']?(?:[a-z0-9_]*token|[a-z0-9_]*password|passwd|secret|api[_-]?key|access[_-]?key|client[_-]?secret|x-amz-signature|x-amz-credential|sig|signature|sharedaccesssignature)["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s&,;]+)`),
	regexp.MustCompile(`https?://[^\s/@]+:[^\s/@]+@`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
}

var reportUnicodeText = regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)
var reportJSONKey = regexp.MustCompile(`"(?:\\.|[^"\\])*"\s*:`)

func scrubReport(v string, credential secret.Value) string {
	// Canonicalize printable Unicode escapes before exact credential matching,
	// retaining quotes, backslashes and controls so quoted-value boundaries stay intact.
	v = reportUnicodeText.ReplaceAllStringFunc(v, func(m string) string {
		decoded, err := strconv.Unquote(`"` + m + `"`)
		if err != nil || decoded == `"` || decoded == `\` {
			return m
		}
		for _, r := range decoded {
			if r < 32 {
				return m
			}
		}
		return decoded
	})
	// Nested JSON in local logs can quote both the key and its delimiters.
	// Inspect a flattened copy only to detect sensitive material; do not send
	// a partially decoded value whose quoting boundaries may be ambiguous.
	if strings.Contains(v, `\"`) || strings.Contains(v, `\u0022`) {
		probe := strings.ReplaceAll(strings.ReplaceAll(v, `\u0022`, `"`), `\`, "")
		for _, pattern := range reportSecrets {
			if pattern.MatchString(probe) {
				return "[redacted: encoded sensitive content omitted]"
			}
		}
	}
	// Decode escaped key names only. Decoding a value's escaped quotation mark
	// before matching quoted values would turn that mark into a false delimiter.
	v = reportJSONKey.ReplaceAllStringFunc(v, func(m string) string {
		key := strings.TrimSpace(strings.TrimSuffix(m, ":"))
		decoded, err := strconv.Unquote(key)
		if err != nil {
			return m
		}
		encoded, _ := json.Marshal(decoded)
		return string(encoded) + ":"
	})
	if !credential.IsZero() {
		v = strings.ReplaceAll(v, credential.Reveal(), secret.Mask)
		if raw := strings.TrimPrefix(credential.Reveal(), "Bearer "); raw != "" {
			v = strings.ReplaceAll(v, raw, secret.Mask)
		}
	}
	for _, r := range reportSecrets {
		v = r.ReplaceAllString(v, secret.Mask)
	}
	return strings.ToValidUTF8(v, "")
}
func reportExcerpt(v string, n int) string {
	if len(v) <= n {
		return v
	}
	v = v[:n]
	for !utf8.ValidString(v) {
		v = v[:len(v)-1]
	}
	return v
}
func (s *Server) cleanReports() {
	now := time.Now()
	for id, r := range s.reportState.reports {
		if now.Sub(r.created) > reportTTL {
			delete(s.reportState.reports, id)
		}
	}
	for id, e := range s.reportState.events {
		if now.Sub(e.created) > reportTTL {
			delete(s.reportState.events, id)
		}
	}
}
func (s *Server) handlePrepareBugReport(ctx context.Context, req *mcp.CallToolRequest, in prepareBugReportInput) (*mcp.CallToolResult, any, error) {
	fail := func(e error) (*mcp.CallToolResult, any, error) { return errorResult(e), nil, nil }
	// Bound assistant input before fingerprint scanning or sanitation work.
	if len(in.Diagnostics) > 16384 || len(in.Evidence) > 4000 || len(in.Actual) > 4000 || len(in.Expected) > 4000 || len(in.Summary) > 500 {
		return fail(fmt.Errorf("report input exceeds field limits"))
	}
	if strings.TrimSpace(in.Evidence) == "" {
		return fail(fmt.Errorf("evidence must be nonempty"))
	}
	var ec execctx.Context
	occurred := time.Now().UTC()
	outcome := "not_applicable"
	provenance := "Assistant supplied context; original call diagnostics unavailable. Agent logs are available to hosted support."
	if in.EventID != "" {
		if in.Profile != "" {
			return fail(fmt.Errorf("event_id and profile are mutually exclusive"))
		}
		s.reportState.mu.Lock()
		s.cleanReports()
		e, ok := s.reportState.events[in.EventID]
		s.reportState.mu.Unlock()
		if !ok || e.session != reportSession(req) {
			return fail(fmt.Errorf("event not available in this session"))
		}
		ec = e.ec
		for _, p := range []*string{&in.Summary, &in.Expected, &in.Actual, &in.Evidence, &in.Diagnostics, &in.Plugin, &in.ResourceType} {
			*p = scrubReportFingerprints(scrubReport(*p, secret.Value{}), e.fingerprints)
		}
		for k, v := range in.Versions {
			in.Versions[k] = scrubReportFingerprints(scrubReport(v, secret.Value{}), e.fingerprints)
		}
		occurred = e.created
		outcome = e.outcome
		in.Tool = e.tool
		if in.CommandID != "" && in.CommandID != e.commandID {
			return fail(fmt.Errorf("command_id does not match this captured event; query the individual command first"))
		}
		in.CommandID = e.commandID
		if in.Outcome != "" {
			switch in.Outcome {
			case "not_sent", "unknown", "partial", "failed", "not_applicable":
				outcome = in.Outcome
			default:
				return fail(fmt.Errorf("invalid outcome"))
			}
		}
		in.Diagnostics = reportExcerpt(e.diagnostics+"\n"+scrubReport(in.Diagnostics, secret.Value{}), 16384)
		provenance = "Captured event " + in.EventID + ". Agent logs are available to hosted support."
	} else {
		if in.Profile == "" {
			return fail(fmt.Errorf("provide event_id or an explicit hosted profile for assistant supplied context"))
		}
		var err error
		ec, err = s.resolveCtx(ctx, in.Profile)
		if err != nil {
			return fail(err)
		}
	}
	if in.EventID == "" {
		if in.OccurredAt != "" {
			parsed, err := time.Parse(time.RFC3339, in.OccurredAt)
			if err != nil {
				return fail(fmt.Errorf("occurred_at must be RFC3339"))
			}
			occurred = parsed
		} else {
			provenance += " Failure time unavailable; occurred_at is preparation time."
		}
		if in.Outcome != "" {
			outcome = in.Outcome
		}
		switch outcome {
		case "not_sent", "unknown", "partial", "failed", "not_applicable":
		default:
			return fail(fmt.Errorf("invalid outcome"))
		}
	}
	h, ok := ec.Conn.(config.Hosted)
	if !ok {
		return fail(fmt.Errorf("%s", classicReportHelp))
	}
	if err := config.ValidateHosted(h); err != nil {
		return fail(err)
	}
	if ec.ProfileName == "" {
		return fail(fmt.Errorf("resolved hosted profile has no name"))
	}
	if in.Component != "formae" && in.Component != "plugin" && in.Component != "mcp" && in.Component != "unknown" {
		return fail(fmt.Errorf("invalid component"))
	}
	r := bugReport{SchemaVersion: 1, ReportID: reportUUID(), OccurredAt: occurred.Format(time.RFC3339), Tool: in.Tool, Component: in.Component, Summary: in.Summary, Expected: in.Expected, Actual: in.Actual, Evidence: in.Evidence + "\n" + provenance, Outcome: outcome, CommandID: in.CommandID, ResourceType: in.ResourceType, Plugin: in.Plugin, ProfileName: ec.ProfileName, Versions: map[string]string{"mcp": version.String()}, Diagnostics: in.Diagnostics}
	for k, v := range in.Versions {
		if k != "cli" && k != "agent" && k != "plugin" {
			return fail(fmt.Errorf("versions accepts cli, agent and plugin only"))
		}
		v = scrubReport(v, ec.Credential)
		if strings.ContainsRune(v, 0) {
			return fail(fmt.Errorf("version must not contain NUL"))
		}
		if strings.TrimSpace(v) == "" {
			return fail(fmt.Errorf("version must not be blank"))
		}
		if len(v) > 128 {
			return fail(fmt.Errorf("version exceeds 128 bytes"))
		}
		if v != "" {
			r.Versions[k] = v
		}
	}
	missing := []string{}
	for _, k := range []string{"cli", "agent", "plugin"} {
		if r.Versions[k] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		r.Evidence += " Missing versions: " + strings.Join(missing, ", ") + "."
	}
	fields := []struct {
		p        *string
		n        int
		required bool
	}{{&r.Tool, 128, true}, {&r.Summary, 500, true}, {&r.Expected, 4000, true}, {&r.Actual, 4000, true}, {&r.Evidence, 4000, true}, {&r.CommandID, 256, false}, {&r.ResourceType, 256, false}, {&r.Plugin, 256, false}, {&r.ProfileName, 256, false}, {&r.Diagnostics, 16384, false}}
	for _, f := range fields {
		*f.p = scrubReport(*f.p, ec.Credential)
		if f.required && strings.TrimSpace(*f.p) == "" {
			return fail(fmt.Errorf("tool, summary, expected, actual and evidence must be nonempty"))
		}
		if strings.ContainsRune(*f.p, 0) {
			return fail(fmt.Errorf("report fields must not contain NUL"))
		}
		if *f.p != "" && f.p != &r.Diagnostics && strings.TrimSpace(*f.p) == "" {
			return fail(fmt.Errorf("report metadata must not be blank"))
		}
		if len(*f.p) > f.n {
			return fail(fmt.Errorf("report field exceeds %d UTF-8 bytes", f.n))
		}
	}
	if strings.TrimSpace(r.Diagnostics) == "" {
		r.Diagnostics = "No relevant local CLI diagnostics available."
	}
	body, err := json.Marshal(r)
	if err != nil {
		return fail(err)
	}
	if len(body) > 65536 {
		return fail(fmt.Errorf("report exceeds 64 KiB"))
	}
	ec.Credential = secret.Value{}
	s.reportState.mu.Lock()
	defer s.reportState.mu.Unlock()
	s.cleanReports()
	if len(s.reportState.reports) >= maxStoredReports {
		return fail(fmt.Errorf("session report storage is full; reports expire after one hour"))
	}
	if s.reportState.reports == nil {
		s.reportState.reports = make(map[string]*storedReport)
	}
	s.reportState.reports[r.ReportID] = &storedReport{session: reportSession(req), created: time.Now(), ec: ec, body: body}
	preview, _ := json.Marshal(struct {
		ReportID        string          `json:"report_id"`
		Recipient       string          `json:"recipient"`
		Report          json.RawMessage `json:"report"`
		Lifetime        string          `json:"lifetime"`
		InstallationID  string          `json:"installation_id"`
		ContactMetadata string          `json:"contact_metadata"`
	}{r.ReportID, supportRecipient, body, "This MCP session only; expires after one hour.", h.Installation, "Support also receives authenticated account contact and installation metadata."})
	return jsonResult(preview), nil, nil
}
func (s *Server) handleSubmitBugReport(ctx context.Context, req *mcp.CallToolRequest, in submitBugReportInput) (*mcp.CallToolResult, any, error) {
	fail := func(e error) (*mcp.CallToolResult, any, error) { return errorResult(e), nil, nil }
	if !in.Confirmed {
		return fail(fmt.Errorf("sending requires user authorization and confirmed=true"))
	}
	s.reportState.mu.Lock()
	s.cleanReports()
	r, ok := s.reportState.reports[in.ReportID]
	s.reportState.mu.Unlock()
	if !ok || r.session != reportSession(req) {
		return fail(fmt.Errorf("report not available in this session; prepare a new preview"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.receipt != nil {
		receipt := *r.receipt
		receipt.Duplicate = true
		b, _ := json.Marshal(receipt)
		return jsonResult(b), nil, nil
	}
	ec, err := s.ctxResolver.Resolve(ctx, r.ec.ProfileName, true)
	if err != nil {
		return fail(fmt.Errorf("could not refresh report credential; prepared report retained"))
	}
	h, ok := ec.Conn.(config.Hosted)
	old := r.ec.Conn.(config.Hosted)
	if !ok || h != old || ec.ProfileName != r.ec.ProfileName {
		return fail(errConnectionMoved)
	}
	if err := config.ValidateHosted(h); err != nil {
		return fail(err)
	}
	if ec.Credential.IsZero() {
		return fail(fmt.Errorf("no hosted credential; report retained"))
	}
	// Never silently change an approved preview when a refreshed credential is discovered in it.
	var decoded bugReport
	_ = json.Unmarshal(r.body, &decoded)
	for _, v := range []string{decoded.Tool, decoded.Summary, decoded.Expected, decoded.Actual, decoded.Evidence, decoded.Diagnostics, decoded.Plugin, decoded.ResourceType, decoded.CommandID, decoded.ProfileName, decoded.Versions["mcp"], decoded.Versions["cli"], decoded.Versions["agent"], decoded.Versions["plugin"]} {
		if scrubReport(v, ec.Credential) != v {
			return fail(fmt.Errorf("fresh credential changes sanitization; prepare a new preview before sending"))
		}
	}
	request, err := newBugReportRequest(ctx, h.Installation, r.body)
	if err != nil {
		return fail(err)
	}
	request.Header.Set("Authorization", ec.Credential.Reveal())
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", in.ReportID)
	client := &http.Client{Timeout: 30 * time.Second, Transport: s.reportTransport, CheckRedirect: refuseRedirects}
	resp, err := client.Do(request)
	if err != nil {
		return fail(fmt.Errorf("report delivery could not be confirmed; prepared report retained; retry only this report_id"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 202 {
		return fail(fmt.Errorf("support endpoint returned HTTP %d; prepared report retained", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(body) > 4096 {
		return fail(fmt.Errorf("report receipt unavailable; delivery unknown; prepared report retained"))
	}
	var receipt bugReceipt
	if json.Unmarshal(body, &receipt) != nil || receipt.ReportID != in.ReportID || receipt.Recipient != supportRecipient || (resp.StatusCode == 202 && receipt.Status != "delivery_unknown") || (resp.StatusCode != 202 && receipt.Status != "sent") {
		return fail(fmt.Errorf("invalid support receipt; delivery unknown; prepared report retained"))
	}
	if receipt.Status == "sent" {
		r.receipt = &receipt
	}
	b, _ := json.Marshal(receipt)
	return jsonResult(b), nil, nil
}
