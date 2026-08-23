package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// Connecting a cloud account is two independent invocations, and that is the
// whole shape of this file.
//
// `connect_cloud_account` computes a CloudFormation console URL and exits. The
// operator applies the stack in their own browser, under their own admin
// session, which is where the only mutation happens. `register_cloud_role`
// records the resulting role ARN in a separate process later.
//
// So there is nothing held between the two, and none of login.go's machinery is
// wanted here — no pending slot, no shared scanner, no child outliving its call.
// login needs those because the auth plugin's pending sign-in is in-process
// memory with a single slot; connect has no equivalent, and copying the pattern
// would import a lifecycle for state that does not exist.

// connectSchemaVersion is the document shape this build understands. Checked
// before any other field, so a document from a newer formae is an error rather
// than a guess.
const connectSchemaVersion = 2

// statusAlreadyRegistered is the idempotent case: the row already names this
// exact role, which is what makes re-running the journey harmless.
const statusAlreadyRegistered = "already_registered"

// linksDoc is what a quick-create emit reports: everything a caller needs to
// drive the console and come back with the role ARN.
type linksDoc struct {
	SchemaVersion   int      `json:"schemaVersion"`
	Phase           string   `json:"phase"`
	Cloud           string   `json:"cloud"`
	Account         string   `json:"account"`
	Installation    string   `json:"installation"`
	StackURL        string   `json:"stackUrl"`
	ExpectedRoleArn string   `json:"expectedRoleArn"`
	TemplateSha256  string   `json:"templateSha256"`
	CreateProvider  bool     `json:"createProvider"`
	ResumeCommand   string   `json:"resumeCommand"`
	Warnings        []string `json:"warnings"`
}

// handleConnectCloudAccount computes the console link that connects one cloud
// account to the active installation.
//
// It mutates nothing: the CloudFormation stack is applied by the user, in their
// browser. That is why this tool is not marked destructive.
func (s *Server) handleConnectCloudAccount(ctx context.Context, _ *mcp.CallToolRequest, input tools.ConnectCloudAccountInput) (*mcp.CallToolResult, any, error) {
	args := []string{"connect", "aws", "--account", input.Account, "--quick-create"}
	if input.ProviderExists {
		args = append(args, "--provider-exists")
	}
	args = append(args, "--no-input", "--output-consumer", "machine", "--output-schema", "json")

	out, err := s.runConnect(ctx, args)
	if err != nil {
		return errorResult(err), nil, nil
	}

	var doc linksDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return errorResult(errUnreadableConnect), nil, nil
	}
	if doc.SchemaVersion != connectSchemaVersion || doc.Phase != "links" {
		return errorResult(errUnreadableConnect), nil, nil
	}

	return textResult(renderLinks(doc)), nil, nil
}

// renderLinks tells the caller what to put in front of the user.
func renderLinks(d linksDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Show the user this link and wait for them to apply the stack:\n\n  %s\n\n", d.StackURL)
	fmt.Fprintf(&b, "It creates a role formae will assume: %s\n", d.ExpectedRoleArn)
	if d.CreateProvider {
		b.WriteString("The stack also creates the OIDC identity provider, which this account does not have yet.\n")
	}
	for _, w := range d.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s\n", w)
	}
	b.WriteString("\nWhen they say the stack reached CREATE_COMPLETE, call register_cloud_role " +
		"with that role ARN. Do not run any command yourself.")
	return b.String()
}

// errUnreadableConnect is a document this build could not read.
var errUnreadableConnect = errors.New("formae connect produced output this build could not read; " +
	"the connected formae may be older than this plugin")

// runConnect drives one connect invocation and returns its document.
//
// Bound to the tool call's context, unlike login's child: this process emits its
// document and exits, so there is nothing to outlive the call.
func (s *Server) runConnect(ctx context.Context, args []string) ([]byte, error) {
	cmd := commandWithContext(ctx, s.ctxResolver.Bin(), args...)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, decodeConnectFailure(out, exit.ExitCode())
		}
		return nil, fmt.Errorf("run %s: %w", s.ctxResolver.Bin(), err)
	}
	return out, nil
}

// connectFailureSchemaVersion is the failure envelope shape this build
// understands, checked before any other field.
const connectFailureSchemaVersion = 2

// declaredConnectFailureCodes is the closed namespace the producer promises
// for a declared connect failure. A code outside it is a protocol mismatch,
// not a message to pass along.
var declaredConnectFailureCodes = map[string]bool{
	"untrusted_issuer":       true,
	"control_plane_too_old":  true,
	"installation_not_ready": true,
	"registration_conflict":  true,
	"unsupported_partition":  true,
	"hosted_required":        true,
	"not_authorized":         true,
	"account_mismatch":       true,
	"auth_failed":            true,
}

// connectFailureView is the envelope the producer emits on stdout when a
// connect invocation fails.
type connectFailureView struct {
	SchemaVersion *int   `json:"schemaVersion"`
	Code          string `json:"code"`
	// Message is decoded so it is visibly accounted for, and deliberately never
	// read: the producer builds it from a plugin error string, and a Pkl
	// failure quotes profile source lines, which for a classic profile can hold
	// an inline password.
	Message string `json:"message"`
}

// decodeConnectFailure turns a non-zero exit into an error the caller can act
// on.
//
// exitStatus names the failure when the envelope cannot be read at all, which
// is a supported path rather than a defensive one: argv the command cannot
// parse fails before the flags that say how to render a failure exist, so it
// exits non-zero with no envelope. The raw bytes never reach the error.
func decodeConnectFailure(stdout []byte, exitStatus int) error {
	unreadable := fmt.Errorf("formae connect failed (exit %d)", exitStatus)

	var v connectFailureView
	dec := json.NewDecoder(bytes.NewReader(stdout))
	if err := dec.Decode(&v); err != nil {
		return unreadable
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return unreadable
	}
	if v.SchemaVersion == nil || *v.SchemaVersion != connectFailureSchemaVersion {
		return unreadable
	}
	if !declaredConnectFailureCodes[v.Code] {
		return unreadable
	}

	return errors.New(describeConnectFailure(v.Code))
}

// describeConnectFailure renders a declared code as the MCP's own text.
func describeConnectFailure(code string) string {
	switch code {
	case "untrusted_issuer":
		return "this profile's hosted connection names an issuer this build will not authenticate against"
	case "control_plane_too_old":
		return "the connected control plane is too old to support connect; upgrade it and try again"
	case "installation_not_ready":
		return "the installation is not ready to accept a connect yet; try again shortly"
	case "registration_conflict":
		return "another installation has already registered a different role for this account"
	case "unsupported_partition":
		return "this AWS partition is not supported"
	case "hosted_required":
		return "connect needs a hosted profile; switch to one and try again"
	case "not_authorized":
		return "you are not authorized to connect this account"
	case "account_mismatch":
		return "the account does not match what this connect invocation expected"
	case "auth_failed":
		return "formae could not authenticate for this connect operation"
	default:
		return "formae could not complete the connect operation"
	}
}

// registeredDoc is what a registration reports.
type registeredDoc struct {
	SchemaVersion int      `json:"schemaVersion"`
	Phase         string   `json:"phase"`
	Status        string   `json:"status"`
	Cloud         string   `json:"cloud"`
	Account       string   `json:"account"`
	RoleArn       string   `json:"roleArn"`
	Warnings      []string `json:"warnings"`
}

// handleRegisterCloudRole records the role an applied stack produced.
func (s *Server) handleRegisterCloudRole(ctx context.Context, _ *mcp.CallToolRequest, input tools.RegisterCloudRoleInput) (*mcp.CallToolResult, any, error) {
	out, err := s.runConnect(ctx, []string{
		"connect", "aws", "--account", input.Account, "--role-arn", input.RoleArn,
		"--no-input", "--output-consumer", "machine", "--output-schema", "json",
	})
	if err != nil {
		return errorResult(err), nil, nil
	}

	var doc registeredDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return errorResult(errUnreadableConnect), nil, nil
	}
	if doc.SchemaVersion != connectSchemaVersion || doc.Phase != "registered" {
		return errorResult(errUnreadableConnect), nil, nil
	}

	return textResult(renderRegistered(doc)), nil, nil
}

// renderRegistered reports what the registration did.
//
// It states the fact and stops. There is no verified state to be waiting for:
// the declared-to-verified lifecycle was cut rather than deferred, and the
// control plane has no status field to poll. Failure surfaces where the work
// happens — a command that cannot assume the role fails loudly and the agent
// logs say why — which tells the user everything a verified stamp would, at the
// moment it matters.
func renderRegistered(d registeredDoc) string {
	var b strings.Builder
	if d.Status == statusAlreadyRegistered {
		fmt.Fprintf(&b, "Account %s was already connected to this installation with the same role.\n", d.Account)
	} else {
		fmt.Fprintf(&b, "Connected account %s.\n", d.Account)
	}
	fmt.Fprintf(&b, "Role: %s\n", d.RoleArn)
	for _, w := range d.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s\n", w)
	}
	return b.String()
}

// cloudConnection is one registered row in a connections listing. RoleArn is
// absent for a cloud that has no such concept (only AWS has one today).
type cloudConnection struct {
	Cloud   string `json:"cloud"`
	Account string `json:"account"`
	RoleArn string `json:"roleArn,omitempty"`
}

// connectionsDoc is what `formae connect list` reports.
//
// SchemaVersion and Complete are pointers so a document missing either field
// is distinguishable from one that spells it out as zero/false: json.Unmarshal
// treats a missing field and an explicit zero value identically, and here that
// difference is load-bearing. Complete in particular is the whole point of
// this document: a caller decides whether to offer provisioning on this
// answer, so an absent field must fail validation rather than default to
// "false" and read as "definitely incomplete" when it might just be a
// malformed producer.
type connectionsDoc struct {
	SchemaVersion *int              `json:"schemaVersion"`
	Phase         string            `json:"phase"`
	Installation  string            `json:"installation"`
	Complete      *bool             `json:"complete"`
	Connections   []cloudConnection `json:"connections"`
	Warnings      []string          `json:"warnings"`
}

// connectionsPhase is the phase value a connections listing carries.
const connectionsPhase = "connections"

// errUnreadableConnections is a connections document this build could not
// read.
var errUnreadableConnections = errors.New("formae connect list produced output this build could not read; " +
	"the connected formae may be older than this plugin")

// decodeConnectionsDoc validates a connections document rather than trusting
// json.Unmarshal: a document missing every field this handler cares about
// decodes cleanly into a zero value, and rendering that zero value would
// silently claim a complete, empty listing.
func decodeConnectionsDoc(out []byte) (connectionsDoc, error) {
	var d connectionsDoc
	dec := json.NewDecoder(bytes.NewReader(out))
	if err := dec.Decode(&d); err != nil {
		return connectionsDoc{}, errUnreadableConnections
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return connectionsDoc{}, errUnreadableConnections
	}
	if d.SchemaVersion == nil || *d.SchemaVersion != connectSchemaVersion {
		return connectionsDoc{}, errUnreadableConnections
	}
	if d.Phase != connectionsPhase {
		return connectionsDoc{}, errUnreadableConnections
	}
	if d.Complete == nil {
		return connectionsDoc{}, errUnreadableConnections
	}
	return d, nil
}

// handleListCloudConnections reports which cloud accounts this installation
// has registered, so a caller (the setup skill, in particular) can decide
// whether to offer the connect flow.
func (s *Server) handleListCloudConnections(ctx context.Context, _ *mcp.CallToolRequest, _ tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	bin := s.formaeBin()
	if err := featuregate.GuardFeatureContext(ctx, featuregate.FeatureCloudConnectionList, bin); err != nil {
		return errorResult(err), nil, nil
	}

	out, err := s.runConnect(ctx, []string{"connect", "list", "--output-consumer", "machine", "--output-schema", "json"})
	if err != nil {
		return errorResult(err), nil, nil
	}

	doc, err := decodeConnectionsDoc(out)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return textResult(renderConnections(doc)), nil, nil
}

// renderConnections tells the caller what is registered, or that it could not
// tell.
//
// An incomplete listing is reported as "cannot be determined", never as an
// empty one: a caller that reads "no accounts" out of an incomplete answer
// would send someone with a working connection through provisioning again.
func renderConnections(d connectionsDoc) string {
	var b strings.Builder
	if !*d.Complete {
		b.WriteString("Whether any cloud account is registered could not be determined: the listing did not " +
			"complete. This is NOT the same as no account being registered; do not offer the connect flow on " +
			"this answer alone.\n")
	} else if len(d.Connections) == 0 {
		b.WriteString("No cloud account is registered yet.\n")
	}
	for _, c := range d.Connections {
		fmt.Fprintf(&b, "%s account %s is registered", c.Cloud, c.Account)
		if c.RoleArn != "" {
			fmt.Fprintf(&b, " (role %s)", c.RoleArn)
		}
		b.WriteString("\n")
	}
	for _, w := range d.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s\n", w)
	}
	return b.String()
}
