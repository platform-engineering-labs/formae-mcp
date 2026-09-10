package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
)

type reportTransport func(*http.Request) (*http.Response, error)

func (f reportTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reportServer(t *testing.T) *Server {
	s := serverWithStubResolver(t, execctx.Context{ProfileName: "original", Conn: config.Hosted{Endpoint: config.HostedOrigin, Installation: "3IzNhVWTOwLD9D8HtLtjq9Jb8ic"}, Credential: secret.New("Bearer secret-token")})
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("unexpected report submission")
		}
		return reportRecipientResponse(), nil
	})
	return s
}
func reportRecipientResponse() *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"recipient":"support@platform.engineering"}`))}
}

func reportInput() prepareBugReportInput {
	return prepareBugReportInput{Profile: "original", Tool: "apply", Component: "formae", Summary: "failure", Expected: "success", Actual: "failed", Evidence: "local failure", Diagnostics: "password=hunter2 Authorization: Bearer secret-token"}
}
func prepared(t *testing.T, s *Server, req *mcp.CallToolRequest) (string, json.RawMessage) {
	t.Helper()
	r, _, _ := s.handlePrepareBugReport(context.Background(), req, reportInput())
	if r.IsError {
		t.Fatal(r.Content)
	}
	var v struct {
		ReportID string          `json:"report_id"`
		Report   json.RawMessage `json:"report"`
	}
	if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &v); err != nil {
		t.Fatal(err)
	}
	return v.ReportID, v.Report
}
func TestBugReportPreviewFrozenAndSessionScoped(t *testing.T) {
	s := reportServer(t)
	calls := 0
	var body []byte
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return reportRecipientResponse(), nil
		}
		calls++
		body, _ = io.ReadAll(r.Body)
		if r.URL.Host != "console.formae.ai" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("wrong endpoint/auth")
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"report_id":"` + r.Header.Get("Idempotency-Key") + `","status":"sent","recipient":"support@platform.engineering","duplicate":false}`)), Header: http.Header{}}, nil
	})
	req := &mcp.CallToolRequest{Session: new(mcp.ServerSession)}
	id, preview := prepared(t, s, req)
	if calls != 0 || strings.Contains(string(preview), "hunter2") || strings.Contains(string(preview), "secret-token") {
		t.Fatalf("unsafe prepare %s", preview)
	}
	r, _, _ := s.handleSubmitBugReport(context.Background(), &mcp.CallToolRequest{Session: new(mcp.ServerSession)}, submitBugReportInput{ReportID: id, Confirmed: true})
	if !r.IsError {
		t.Fatal("cross session accepted")
	}
	r, _, _ = s.handleSubmitBugReport(context.Background(), req, submitBugReportInput{ReportID: id})
	if !r.IsError || calls != 0 {
		t.Fatal("unconfirmed accepted")
	}
	r, _, _ = s.handleSubmitBugReport(context.Background(), req, submitBugReportInput{ReportID: id, Confirmed: true})
	if r.IsError {
		t.Fatal(r.Content)
	}
	if string(body) != string(preview) {
		t.Fatalf("preview changed: %s vs %s", preview, body)
	}
	r, _, _ = s.handleSubmitBugReport(context.Background(), req, submitBugReportInput{ReportID: id, Confirmed: true})
	if r.IsError {
		t.Fatal(r.Content)
	}
	if calls != 1 {
		t.Fatal("duplicate sent")
	}
}
func TestBugReportRejectClassicAndMoved(t *testing.T) {
	s := reportServer(t)
	id, _ := prepared(t, s, nil)
	s.ctxResolver.(*stubResolver).ec.Conn = config.Classic{URL: "http://localhost"}
	r, _, _ := s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: id, Confirmed: true})
	if !r.IsError {
		t.Fatal("moved accepted")
	}
	r, _, _ = s.handlePrepareBugReport(context.Background(), nil, reportInput())
	if !r.IsError {
		t.Fatal("classic accepted")
	}
}
func TestBugReportCapturesFailedStatusOriginalContext(t *testing.T) {
	s := reportServer(t)
	req := &mcp.CallToolRequest{Session: new(mcp.ServerSession), Params: &mcp.CallToolParamsRaw{Name: "get_command_status"}}
	handler := s.captureReportEvent(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		_, err := s.resolveCtx(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		return textResult(`{"Commands":[{"CommandId":"command-1","State":"Failed","ResourceUpdates":[{"ErrorMessage":"plugin crashed","Properties":{"password":"inventory-secret"}}]}]}`), nil
	})
	result, err := handler(context.Background(), "tools/call", req)
	if err != nil {
		t.Fatal(err)
	}
	r := result.(*mcp.CallToolResult)
	if len(r.Content) != 2 {
		t.Fatal("missing event reference")
	}
	id := strings.TrimPrefix(r.Content[1].(*mcp.TextContent).Text, "Bug report event_id: ")
	// Simulate the active pointer moving while a refresh of the original
	// named profile continues to resolve its original installation.
	resolver := s.ctxResolver.(*stubResolver)
	original := resolver.ec
	resolver.refreshed = &original
	resolver.ec.Conn = config.Classic{URL: "http://localhost"}
	in := reportInput()
	in.EventID = id
	in.Profile = ""
	in.Diagnostics = ""
	r, _, _ = s.handlePrepareBugReport(context.Background(), req, in)
	if resolver.sawProfile != "original" {
		t.Fatal("discovery used active profile")
	}
	if r.IsError {
		t.Fatal(r.Content)
	}
	body := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(body, `"outcome":"failed"`) || !strings.Contains(body, "command-1") || strings.Contains(body, "inventory-secret") {
		t.Fatal(body)
	}
}

func TestBugReportFallbackMetadataAndEvidence(t *testing.T) {
	s := reportServer(t)
	in := reportInput()
	in.Evidence = ""
	r, _, _ := s.handlePrepareBugReport(context.Background(), nil, in)
	if !r.IsError {
		t.Fatal("empty evidence accepted")
	}
	in = reportInput()
	in.OccurredAt = "2026-09-01T12:00:00Z"
	in.Outcome = "partial"
	in.Versions = map[string]string{"cli": "0.89.0"}
	r, _, _ = s.handlePrepareBugReport(context.Background(), nil, in)
	if r.IsError {
		t.Fatal(r.Content)
	}
	body := r.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"2026-09-01T12:00:00Z", `"outcome":"partial"`, `"cli":"0.89.0"`} {
		if !strings.Contains(body, want) {
			t.Fatal(body)
		}
	}
}
func TestBugReportDeliveryUnknownAndRedirects(t *testing.T) {
	for _, status := range []int{202, 302, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := reportServer(t)
			id, _ := prepared(t, s, nil)
			calls := 0
			s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://evil.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"report_id":"` + id + `","status":"delivery_unknown","recipient":"support@platform.engineering","duplicate":false}`))}, nil
			})
			r, _, _ := s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: id, Confirmed: true})
			if (status == 202) == r.IsError {
				t.Fatal(r.Content)
			}
			if calls != 1 {
				t.Fatal("redirect or automatic retry")
			}
			if status == 202 && !strings.Contains(r.Content[0].(*mcp.TextContent).Text, "delivery_unknown") {
				t.Fatal(r.Content)
			}
		})
	}
}
func TestBugReportConcurrentSubmission(t *testing.T) {
	s := reportServer(t)
	id, _ := prepared(t, s, nil)
	var calls atomic.Int32
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond)
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"report_id":"` + id + `","status":"sent","recipient":"support@platform.engineering","duplicate":false}`))}, nil
	})
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _, _ := s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: id, Confirmed: true})
			if r.IsError {
				t.Error(r.Content)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestBugReportExpiredAndCredentialChange(t *testing.T) {
	s := reportServer(t)
	in := reportInput()
	in.Summary = "new-credential"
	r, _, _ := s.handlePrepareBugReport(context.Background(), nil, in)
	var preview struct {
		ReportID string `json:"report_id"`
	}
	if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &preview); err != nil {
		t.Fatal(err)
	}
	s.ctxResolver.(*stubResolver).ec.Credential = secret.New("Bearer new-credential")
	r, _, _ = s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: preview.ReportID, Confirmed: true})
	if !r.IsError {
		t.Fatal("changed sanitizer accepted")
	}
	s.reportState.reports[preview.ReportID].created = time.Now().Add(-2 * time.Hour)
	r, _, _ = s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: preview.ReportID, Confirmed: true})
	if !r.IsError {
		t.Fatal("expired accepted")
	}
}
func TestBugReportScrubsSensitiveFormats(t *testing.T) {
	for _, raw := range []string{"password=hunter2", "Authorization: Bearer hunter2", "Cookie: session=hunter2", `{"access_token":"hunter2"}`, "https://user:hunter2@example.org", "-----BEGIN PRIVATE KEY-----\nhunter2\n-----END PRIVATE KEY-----", "token=hunter2"} {
		if strings.Contains(scrubReport(raw, secret.Value{}), "hunter2") {
			t.Fatal(raw)
		}
	}
}
func TestBugReportOriginalCredentialScrubbedInSupplement(t *testing.T) {
	s := reportServer(t)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "extract_resources"}}
	handler := s.captureReportEvent(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		if _, err := s.resolveCtx(ctx, ""); err != nil {
			t.Fatal(err)
		}
		captureReportLocalDiagnostics(ctx, "local window password=hunter2")
		return errorResult(fmt.Errorf("conversion failed")), nil
	})
	result, _ := handler(context.Background(), "tools/call", req)
	r := result.(*mcp.CallToolResult)
	id := strings.TrimPrefix(r.Content[1].(*mcp.TextContent).Text, "Bug report event_id: ")
	in := reportInput()
	in.EventID = id
	in.Profile = ""
	in.Diagnostics = "old raw secret-token"
	r, _, _ = s.handlePrepareBugReport(context.Background(), req, in)
	if r.IsError {
		t.Fatal(r.Content)
	}
	text := r.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "secret-token") || strings.Contains(text, "hunter2") || !strings.Contains(text, "local window") {
		t.Fatal(text)
	}
}
func TestBugReportToolsThroughMCPSession(t *testing.T) {
	s := reportServer(t)
	ctx := context.Background()
	a, b := mcp.NewInMemoryTransports()
	ss, err := s.mcpServer.Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ss.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "bug-report-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "prepare_bug_report", Arguments: reportInput()})
	if err != nil || r.IsError {
		t.Fatalf("prepare %v %v", r, err)
	}
	var preview struct {
		ReportID string `json:"report_id"`
	}
	if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.ReportID == "" {
		t.Fatal("missing id")
	}
	// A caller cannot inject a replacement payload into submission.
	r, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "submit_bug_report", Arguments: map[string]any{"report_id": preview.ReportID, "confirmed": true, "summary": "replacement"}})
	if err == nil && !r.IsError {
		t.Fatal("unknown submit argument accepted")
	}
}
func TestBugReportOptionalMetadataNotWhitespace(t *testing.T) {
	for _, field := range []string{"plugin", "version"} {
		s := reportServer(t)
		in := reportInput()
		if field == "plugin" {
			in.Plugin = "   "
		} else {
			in.Versions = map[string]string{"cli": "   "}
		}
		r, _, _ := s.handlePrepareBugReport(context.Background(), nil, in)
		if !r.IsError {
			t.Fatalf("blank %s accepted", field)
		}
	}
}
func TestBugReportFailureCommandPairing(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(`{"Commands":[{"CommandId":"failed-A","State":"Failed","ResourceUpdates":[{"State":"Success"},{"State":"Failed","ErrorMessage":"failed-a"}]},{"CommandId":"success-B","State":"Success"}]}`), &v); err != nil {
		t.Fatal(err)
	}
	f, id, e := reportFailureFields(v)
	if !f || id != "failed-A" || !strings.Contains(e, "failed-a") {
		t.Fatalf("%v %s %s", f, id, e)
	}
}
func TestBugReportEscapedJSONSecrets(t *testing.T) {
	for _, raw := range []string{`{"password":"prefix\"sensitive-tail"}`, `{"pass\u0077ord":"sensitive-tail"}`} {
		if strings.Contains(scrubReport(raw, secret.Value{}), "sensitive-tail") {
			t.Fatal(raw)
		}
	}
}
func TestBugReportClassicNoEvent(t *testing.T) {
	s := reportServer(t)
	s.ctxResolver.(*stubResolver).ec.Conn = config.Classic{URL: "http://localhost"}
	r, _ := s.captureReportEvent(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		if _, err := s.resolveCtx(ctx, ""); err != nil {
			t.Fatal(err)
		}
		return errorResult(fmt.Errorf("failed")), nil
	})(context.Background(), "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "apply"}})
	if len(r.(*mcp.CallToolResult).Content) != 1 {
		t.Fatal("classic report reference")
	}
}
func TestBugReportPartialAndPreviewDestination(t *testing.T) {
	var v any
	if err := json.Unmarshal([]byte(`{"Commands":[{"CommandId":"A","State":"Failed","ResourceUpdates":[{"State":"Success"},{"State":"Rejected"}]}]}`), &v); err != nil {
		t.Fatal(err)
	}
	if !reportPartialFailure(v) {
		t.Fatal("partial completion lost")
	}
	s := reportServer(t)
	r, _, _ := s.handlePrepareBugReport(context.Background(), nil, reportInput())
	body := r.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(body, `"installation_id":"3IzNhVWTOwLD9D8HtLtjq9Jb8ic"`) || !strings.Contains(body, "authenticated account contact") {
		t.Fatal(body)
	}
	in := reportInput()
	in.Actual = "bad\x00value"
	r, _, _ = s.handlePrepareBugReport(context.Background(), nil, in)
	if !r.IsError {
		t.Fatal("NUL accepted")
	}
}
func TestBugReportSignedURLAndCloudSecrets(t *testing.T) {
	for _, raw := range []string{`{"SecretAccessKey":"sensitive-tail"}`, "aws_secret_access_key=sensitive-tail", "https://example.org/?X-Amz-Signature=sensitive-tail&other=ok", "https://example.org/?sig=sensitive-tail&other=ok", `{"password":"prefix\u0022sensitive-tail"}`} {
		got := scrubReport(raw, secret.Value{})
		if strings.Contains(got, "sensitive-tail") {
			t.Fatal(raw)
		}
		if twice := scrubReport(got, secret.Value{}); twice != got {
			t.Fatalf("not idempotent: %q vs %q", got, twice)
		}
	}
}
func TestBugReportEncodedOriginalCredential(t *testing.T) {
	secretValue := secret.New("Bearer original-token")
	raw := `diagnostic original-\u0074oken`
	got := scrubReportFingerprints(scrubReport(raw, secret.Value{}), reportFingerprints(secretValue))
	if strings.Contains(got, "original-") {
		t.Fatal(got)
	}
	if scrubReport(got, secret.New("Bearer different-token")) != got {
		t.Fatal("changed preview")
	}
}

func TestBugReportNestedEncodedSecrets(t *testing.T) {
	for _, raw := range []string{`level=ERROR body={\"password\":\"hunter2\"}`, `{\u0022password\u0022:\u0022hunter2\u0022}`, `body={\\\"password\\\":\\\"hunter2\\\"}`, `body={\"pass\\u0077ord\":\"hunter2\"}`} {
		got := scrubReport(raw, secret.Value{})
		if strings.Contains(got, "hunter2") {
			t.Fatalf("nested secret leaked: %s", got)
		}
		if again := scrubReport(got, secret.Value{}); again != got {
			t.Fatalf("not idempotent: %s => %s", got, again)
		}
	}
}

func TestBugReportStatusEventsOnlyForSingleCommandStatus(t *testing.T) {
	single := `{"Commands":[{"CommandId":"old-failure","State":"Failed"}]}`
	multi := `{"Commands":[{"CommandId":"old-failure","State":"Failed"},{"CommandId":"other","State":"Success"}]}`
	for _, tc := range []struct {
		name, payload      string
		isError, wantEvent bool
	}{{"list_commands", single, false, false}, {"list_resources", single, false, false}, {"get_command_status", multi, false, false}, {"get_command_status", single, false, true}, {"list_resources", "local failure", true, true}} {
		t.Run(tc.name+fmt.Sprint(tc.isError)+tc.payload, func(t *testing.T) {
			s := reportServer(t)
			handler := s.captureReportEvent(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
				if _, err := s.resolveCtx(ctx, ""); err != nil {
					t.Fatal(err)
				}
				r := textResult(tc.payload)
				r.IsError = tc.isError
				return r, nil
			})
			result, err := handler(context.Background(), "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: tc.name}})
			if err != nil {
				t.Fatal(err)
			}
			r := result.(*mcp.CallToolResult)
			if (len(r.Content) > 1) != tc.wantEvent {
				t.Fatalf("event=%v want=%v", len(r.Content) > 1, tc.wantEvent)
			}
		})
	}
}

func TestBugReportUnknownRetryObservesSent(t *testing.T) {
	s := reportServer(t)
	id, preview := prepared(t, s, nil)
	calls := 0
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return reportRecipientResponse(), nil
		}
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != string(preview) || r.Header.Get("Idempotency-Key") != id {
			t.Fatal("retry identity changed")
		}
		status := 202
		delivery := "delivery_unknown"
		if calls > 1 {
			status = 200
			delivery = "sent"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"report_id":%q,"status":%q,"recipient":"support@platform.engineering","duplicate":true}`, id, delivery)))}, nil
	})
	for _, want := range []string{"delivery_unknown", "sent", "sent"} {
		r, _, _ := s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: id, Confirmed: true})
		if r.IsError || !strings.Contains(r.Content[0].(*mcp.TextContent).Text, `"status":"`+want+`"`) {
			t.Fatal(r.Content)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestBugReportDiscoversAndBindsRecipient(t *testing.T) {
	s := reportServer(t)
	gets, posts := 0, 0
	const recipient = "helpdesk@example.org"
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != consoleURL+"/api/v1/installations/3IzNhVWTOwLD9D8HtLtjq9Jb8ic/bug-reports" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("wrong destination or auth")
		}
		if r.Method == http.MethodGet {
			gets++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"recipient":"` + recipient + `"}`))}, nil
		}
		posts++
		if r.Header.Get("X-Expected-Bug-Report-Recipient") != recipient {
			t.Fatal("recipient not bound")
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"report_id":"` + r.Header.Get("Idempotency-Key") + `","recipient":"` + recipient + `","status":"sent","duplicate":false}`))}, nil
	})
	r, _, _ := s.handlePrepareBugReport(context.Background(), nil, reportInput())
	if r.IsError {
		t.Fatal(r.Content)
	}
	var p struct {
		ReportID  string `json:"report_id"`
		Recipient string `json:"recipient"`
	}
	if err := json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &p); err != nil {
		t.Fatal(err)
	}
	if p.Recipient != recipient || gets != 1 || posts != 0 {
		t.Fatalf("bad preview/discovery %v gets%d posts%d", p, gets, posts)
	}
	r, _, _ = s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: p.ReportID, Confirmed: true})
	if r.IsError || posts != 1 {
		t.Fatal(r.Content)
	}
}
func TestBugReportRecipientChangeRequiresNewPreview(t *testing.T) {
	s := reportServer(t)
	id, _ := prepared(t, s, nil)
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 409, Body: io.NopCloser(strings.NewReader(`{"error":"recipient_changed"}`))}, nil
	})
	r, _, _ := s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: id, Confirmed: true})
	if !r.IsError || !strings.Contains(textContent(t, r), "prepare a new preview") {
		t.Fatal(r.Content)
	}
}

func TestBugReportDiscoveryFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{302, `{"recipient":"support@example.org"}`}, {503, `{}`}, {200, `{"recipient":"User <support@example.org>"}`}, {200, `{"recipient":"support@example.org\r\nBcc: other@example.org"}`}, {200, `{"recipient":""}`}} {
		t.Run(fmt.Sprint(tc.status)+tc.body, func(t *testing.T) {
			s := reportServer(t)
			calls := 0
			s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.Body != nil {
					t.Fatal("discovery sent report content")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://other.example.org"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			r, _, _ := s.handlePrepareBugReport(context.Background(), nil, reportInput())
			if !r.IsError || calls != 1 || len(s.reportState.reports) != 0 {
				t.Fatalf("discovery should fail closed: %v", r.Content)
			}
		})
	}
}
func TestBugReportReceiptMustMatchPreparedRecipient(t *testing.T) {
	s := reportServer(t)
	id, _ := prepared(t, s, nil)
	s.reportTransport = reportTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"report_id":"` + id + `","recipient":"different@example.org","status":"sent","duplicate":false}`))}, nil
	})
	r, _, _ := s.handleSubmitBugReport(context.Background(), nil, submitBugReportInput{ReportID: id, Confirmed: true})
	if !r.IsError {
		t.Fatal("mismatched recipient accepted")
	}
}
