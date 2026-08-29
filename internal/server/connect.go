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
//
// It is 1 while the success documents are 2, and that is not a mistake: the
// envelope carries its own printer.failureSchemaVersion and the two version
// independently. Assuming they matched made this decoder reject every real
// failure and report an exit status instead, which is the behaviour decoding
// exists to replace. Confirmed against a live run, not inferred.
const connectFailureSchemaVersion = 1

// connectFailureDescriptions maps the codes the producer declares to how this
// build describes them. It mirrors printer's Code constants; keep it in step
// when that set grows.
//
// A code missing from here is NOT a protocol mismatch. The namespace is closed
// but it grows, and this map will always lag it, so an unmapped code is named
// verbatim rather than discarded: a code is our own vocabulary and safe to
// surface, and telling the caller which failure happened beats telling them a
// process exited. Only the producer's message is withheld.
var connectFailureDescriptions = map[string]string{
	"untrusted_issuer":       "this profile's hosted connection names an issuer this build will not authenticate against",
	"control_plane_too_old":  "the connected control plane is too old to support connect; upgrade it and try again",
	"installation_not_ready": "the installation is not ready to accept a connect yet; try again shortly",
	"registration_conflict":  "another installation has already registered a different role for this account",
	"unsupported_partition":  "this AWS partition is not supported",
	"hosted_required":        "connect needs a hosted profile; switch to one and try again",
	"not_authorized":         "you are not authorized to connect this account",
	"account_mismatch":       "the account does not match what this connect invocation expected",
	"auth_failed":            "formae could not authenticate for this connect operation",
	"provision_failed":       "formae could not provision the role with those credentials",
	"role_collision":         "a role of the expected name exists that connect does not own; delete it or connect with an existing role ARN",
	"provider_conflict":      "the account's OIDC provider exists but is not configured the way connect expects",
	"sso_login_required":     "the SSO session for those credentials has expired; sign in again and retry",
	"ambiguous_profile":      "more than one profile exists and none was named, so formae cannot tell which installation you meant",
	"no_connection":          "this profile names no connection to work with",
	"plugin_missing":         "a plugin this operation needs is not installed",
	"login_failed":           "signing in failed",
	"sync_incomplete":        "you are signed in, but formae could not bring the hosted profiles up to date",
	"internal":               "formae could not complete this connect operation",

	// GCP. credentials_required carries the command to run in its details, and
	// a description that did not name it would leave the caller with a refusal
	// and no remedy.
	"credentials_required": "no usable Google Cloud credentials on this machine; run `gcloud auth application-default login` and try again",
	"gcloud_missing":       "the gcloud CLI is needed to sign in to Google Cloud and is not installed; install it from https://cloud.google.com/sdk/docs/install and try again",
	"project_unreachable":  "that GCP project could not be read with these credentials; check the project id, and that this account can see it. Signing in again will not help: it returns the same account",
	"api_disabled":         "a Google API this connection needs is not enabled on that project; enable it and try again",
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
	// Details are relayed, unlike Message, but only the keys a code declares in
	// relayedFailureDetails. Each is a value the producer chose under a key it
	// declared, which is what makes a named key from a named code safe where
	// the whole message is not.
	Details map[string]string `json:"details"`
}

// relayedFailureDetails names, per code, the detail keys this build passes
// through to the caller, in the order they are shown.
//
// An allowlist rather than a passthrough. The producer may attach anything it
// likes to any failure, and a blanket relay would make every future detail on
// every future code a disclosure decision nobody made. Adding a key here is
// that decision, taken once and reviewable.
//
// credentials_required is the case that forced this. Where a browser cannot be
// opened, gcloud prints a sign-in URL and waits for a verification code on a
// stdin that is /dev/null under this server, so the run fails having already
// printed the only thing that would let the operator finish. Dropping it left
// the caller with a canned instruction to run a command that fails the same
// way.
var relayedFailureDetails = map[string][]string{
	"credentials_required": {"output"},
	"gcloud_missing":       {"output"},
}

// withRelayedDetails appends a code's declared detail keys to its description.
func withRelayedDetails(desc string, v connectFailureView) string {
	var parts []string
	for _, key := range relayedFailureDetails[v.Code] {
		if val := strings.TrimSpace(v.Details[key]); val != "" {
			parts = append(parts, val)
		}
	}
	if len(parts) == 0 {
		return desc
	}
	return desc + "\n\n" + strings.Join(parts, "\n\n")
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
	if desc, ok := connectFailureDescriptions[v.Code]; ok {
		return errors.New(withRelayedDetails(desc, v))
	}
	// An envelope this build has no prose for still names its code. The code is
	// ours; the message is the producer's and never crosses.
	if v.Code != "" {
		return fmt.Errorf("formae connect failed (%s)", v.Code)
	}
	return unreadable
}

// describeConnectFailure renders a declared code as the MCP's own text. Every
// caller passes a code decodeConnectFailure has already validated against
// connectFailureDescriptions, so the lookup always hits; there is no
// undeclared-code fallback to maintain here as well as there.
func describeConnectFailure(code string) string {
	return connectFailureDescriptions[code]
}

// registeredDoc is what a registration reports.
// registeredDoc is the producer's "registered" document. Each cloud carries
// exactly its own trust coordinate and omits the others, so which field is
// populated follows from Cloud rather than from whichever happens to be
// non-empty.
type registeredDoc struct {
	SchemaVersion int    `json:"schemaVersion"`
	Phase         string `json:"phase"`
	Status        string `json:"status"`
	Cloud         string `json:"cloud"`
	Account       string `json:"account"`
	RoleArn       string `json:"roleArn"`
	// WorkloadIdentityProvider is the GCP coordinate.
	WorkloadIdentityProvider string   `json:"workloadIdentityProvider"`
	Warnings                 []string `json:"warnings"`
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

	doc, err := decodeRegisteredDoc(out)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return textResult(renderRegistered(doc)), nil, nil
}

// decodeRegisteredDoc validates a registered document rather than trusting
// json.Unmarshal: a document missing both discriminators decodes cleanly into
// a zero value. Shared by register_cloud_role and provision_cloud_role, which
// both end at the same "registered" document — one from a two-step flow, one
// from a single invocation that provisions and registers together.
func decodeRegisteredDoc(out []byte) (registeredDoc, error) {
	var doc registeredDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return registeredDoc{}, errUnreadableConnect
	}
	if doc.SchemaVersion != connectSchemaVersion || doc.Phase != "registered" {
		return registeredDoc{}, errUnreadableConnect
	}
	return doc, nil
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
