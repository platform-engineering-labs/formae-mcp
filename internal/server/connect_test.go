package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// Connecting a cloud account is two independent CLI invocations, not one child
// held across two tool calls. `connect_cloud_account` computes a CloudFormation
// console URL and exits; the operator applies the stack in their own browser;
// `register_cloud_role` records the resulting role ARN in a later process. There
// is no pending state between them, which is what separates this from login.

const linksJSON = `{"schemaVersion":2,"phase":"links","cloud":"aws","account":"123456789012",` +
	`"installation":"2abc","stackUrl":"https://console.aws.amazon.com/cloudformation/quickcreate?x=1",` +
	`"expectedRoleArn":"arn:aws:iam::123456789012:role/formae-connect-2abc",` +
	`"templateSha256":"deadbeef","createProvider":true,"resumeCommand":"formae connect aws --account 123456789012 --role-arn ARN"}`

// The stack URL and the role ARN it will produce are what the user acts on, so
// both reach the caller.
func TestConnectCloudAccount_ReturnsTheStackURLAndExpectedRole(t *testing.T) {
	bin := loginStub(t, fmt.Sprintf("echo '%s'\n", linksJSON))
	s := serverWithLoginBin(t, bin)

	res, _, err := s.handleConnectCloudAccount(context.Background(), nil,
		tools.ConnectCloudAccountInput{Account: "123456789012"})
	if err != nil {
		t.Fatalf("connect_cloud_account: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	got := resultText(res)
	for _, want := range []string{
		"https://console.aws.amazon.com/cloudformation/quickcreate?x=1",
		"arn:aws:iam::123456789012:role/formae-connect-2abc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result does not carry %q:\n%s", want, got)
		}
	}
}

// argvStub writes a fake formae that records its arguments to a file and then
// emits doc. Recording argv is the only way to prove a flag that changes what
// the console link does was actually passed.
func argvStub(t *testing.T, argvFile, doc string) string {
	t.Helper()
	return loginStub(t, fmt.Sprintf("echo \"$@\" > %s\necho '%s'\n", argvFile, doc))
}

// A fresh account has no OIDC provider, so the default must create one. Passing
// --provider-exists here would emit a link whose stack skips the provider, and
// the role would trust an identity provider that does not exist.
func TestConnectCloudAccount_DoesNotClaimTheProviderExistsByDefault(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, linksJSON))

	if _, _, err := s.handleConnectCloudAccount(context.Background(), nil,
		tools.ConnectCloudAccountInput{Account: "123456789012"}); err != nil {
		t.Fatalf("connect_cloud_account: %v", err)
	}

	got := readFile(t, argv)
	if strings.Contains(got, "--provider-exists") {
		t.Errorf("argv carries --provider-exists when the caller did not ask for it: %s", got)
	}
	if !strings.Contains(got, "--no-input") {
		t.Errorf("argv does not disable prompts, so the CLI could block on one: %s", got)
	}
}

// An account connected to formae before already has the account-global provider,
// and a stack that tries to create a second one fails.
func TestConnectCloudAccount_PassesProviderExistsWhenTold(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, linksJSON))

	if _, _, err := s.handleConnectCloudAccount(context.Background(), nil,
		tools.ConnectCloudAccountInput{Account: "123456789012", ProviderExists: true}); err != nil {
		t.Fatalf("connect_cloud_account: %v", err)
	}

	if got := readFile(t, argv); !strings.Contains(got, "--provider-exists") {
		t.Errorf("argv does not carry --provider-exists: %s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

const registeredJSON = `{"schemaVersion":2,"phase":"registered","status":"registered_unverified",` +
	`"cloud":"aws","account":"123456789012","roleArn":"arn:aws:iam::123456789012:role/formae-connect-2abc"}`

const alreadyJSON = `{"schemaVersion":2,"phase":"registered","status":"already_registered",` +
	`"cloud":"aws","account":"123456789012","roleArn":"arn:aws:iam::123456789012:role/formae-connect-2abc"}`

// Registration is a separate invocation carrying the ARN the applied stack
// produced.
func TestRegisterCloudRole_ReportsTheRegisteredAccountAndRole(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, registeredJSON))

	res, _, err := s.handleRegisterCloudRole(context.Background(), nil, tools.RegisterCloudRoleInput{
		Account: "123456789012",
		RoleArn: "arn:aws:iam::123456789012:role/formae-connect-2abc",
	})
	if err != nil {
		t.Fatalf("register_cloud_role: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}
	if got := resultText(res); !strings.Contains(got, "123456789012") {
		t.Errorf("result does not name the account:\n%s", got)
	}
	if got := readFile(t, argv); !strings.Contains(got, "--role-arn") {
		t.Errorf("argv does not carry --role-arn, so this was not the register-only path: %s", got)
	}
}

// already_registered is the idempotent 409-same case: the row already names this
// exact role. Reporting it as a failure would make re-running the journey look
// broken when it is the thing that makes re-running safe.
func TestRegisterCloudRole_TreatsAlreadyRegisteredAsSuccess(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", alreadyJSON)))

	res, _, err := s.handleRegisterCloudRole(context.Background(), nil, tools.RegisterCloudRoleInput{
		Account: "123456789012",
		RoleArn: "arn:aws:iam::123456789012:role/formae-connect-2abc",
	})
	if err != nil {
		t.Fatalf("register_cloud_role: %v", err)
	}
	if isError(res) {
		t.Fatalf("already_registered reported as an error: %s", resultText(res))
	}
}

// A verified state does not exist and never will: it was cut, not deferred. A
// build that renders registered_unverified as pending, or invites the caller to
// poll for a verified one, is describing a lifecycle the control plane has no
// field for.
func TestRegisterCloudRole_DoesNotImplyAVerifiedStateIsComing(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", registeredJSON)))

	res, _, err := s.handleRegisterCloudRole(context.Background(), nil, tools.RegisterCloudRoleInput{
		Account: "123456789012",
		RoleArn: "arn:aws:iam::123456789012:role/formae-connect-2abc",
	})
	if err != nil {
		t.Fatalf("register_cloud_role: %v", err)
	}
	for _, forbidden := range []string{"verif", "pending", "not yet active", "poll"} {
		if strings.Contains(strings.ToLower(resultText(res)), forbidden) {
			t.Errorf("result suggests a verified state with %q:\n%s", forbidden, resultText(res))
		}
	}
}

// A declared failure reaches the caller as something it can act on rather than
// an exit code.
func TestRunConnect_RendersADeclaredFailure(t *testing.T) {
	env := `{"schemaVersion":2,"code":"control_plane_too_old","message":"raw"}`
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\nexit 1\n", env)))
	res, _, _ := s.handleConnectCloudAccount(context.Background(), nil,
		tools.ConnectCloudAccountInput{Account: "123456789012"})
	if !isError(res) || !strings.Contains(strings.ToLower(resultText(res)), "too old") {
		t.Errorf("nothing actionable: %s", resultText(res))
	}
}

// The producer's own prose never crosses: it is built from a plugin error
// string, and a Pkl failure quotes profile source lines that can hold an
// inline password.
func TestRunConnect_NeverSurfacesProducerProse(t *testing.T) {
	secret := "inline-password-from-a-source-line"
	env := fmt.Sprintf(`{"schemaVersion":2,"code":"auth_failed","message":%q}`, secret)
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\nexit 1\n", env)))
	res, _, _ := s.handleConnectCloudAccount(context.Background(), nil,
		tools.ConnectCloudAccountInput{Account: "123456789012"})
	if strings.Contains(resultText(res), secret) {
		t.Fatal("the producer's message reached the caller")
	}
}

// serverForConnectionsList builds a server whose formae binary emits doc and
// whose detected version is forced above the list_cloud_connections floor, so
// the call reaches the handler instead of dying at the gate.
func serverForConnectionsList(t *testing.T, doc string) *Server {
	t.Helper()
	// A real dev tag, not a round number: parseParts discards the suffix, so
	// this is the shape the gate actually meets in testing.
	withFakeVersion(t, "0.89.0-dev.9")
	return serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", doc)))
}

// A complete listing names every registered account, which is what a caller
// deciding whether to provision on this answer needs.
func TestListCloudConnections_ReportsRegisteredAccounts(t *testing.T) {
	doc := `{"schemaVersion":2,"phase":"connections","installation":"2abc","complete":true,` +
		`"connections":[{"cloud":"aws","account":"123456789012"}]}`
	s := serverForConnectionsList(t, doc)
	res, _, err := s.handleListCloudConnections(context.Background(), nil, tools.EmptyInput{})
	if err != nil || isError(res) {
		t.Fatalf("%v %s", err, resultText(res))
	}
	if !strings.Contains(resultText(res), "123456789012") {
		t.Errorf("the account is not named: %s", resultText(res))
	}
}

// An incomplete listing must not read as an empty one, and the success path
// that reports it must not be confused with a decode failure either:
// errUnreadableConnections also reads "could not read", so a substring check
// alone would stay green even if decodeConnectionsDoc started rejecting
// complete:false documents outright. Asserting isError is false and matching
// the exact sentence is what tells the two apart.
func TestListCloudConnections_SaysWhenItCannotTell(t *testing.T) {
	doc := `{"schemaVersion":2,"phase":"connections","installation":"2abc","complete":false,"connections":[]}`
	s := serverForConnectionsList(t, doc)
	res, _, err := s.handleListCloudConnections(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("list_cloud_connections: %v", err)
	}
	if isError(res) {
		t.Fatalf("an incomplete listing was reported as a decode failure rather than the incomplete case: %s", resultText(res))
	}
	got := resultText(res)
	if strings.Contains(strings.ToLower(got), "no cloud account is registered") {
		t.Errorf("an incomplete listing was rendered as the empty-complete case: %s", got)
	}
	want := "Whether any cloud account is registered could not be determined: the listing did not complete."
	if !strings.Contains(got, want) {
		t.Errorf("an incomplete listing does not say so: %s", got)
	}
}

// A document missing both discriminators decodes cleanly into a zero value, so
// the handler validates rather than trusting json.Unmarshal.
func TestListCloudConnections_RefusesADocumentItCannotIdentify(t *testing.T) {
	s := serverForConnectionsList(t, `{"connections":[]}`)
	res, _, _ := s.handleListCloudConnections(context.Background(), nil, tools.EmptyInput{})
	if !isError(res) {
		t.Errorf("an unidentifiable document was accepted: %s", resultText(res))
	}
}

// A complete listing with no rows reports that plainly, since the empty case
// is the one a caller acts on to decide whether to offer the connect flow.
func TestListCloudConnections_ReportsNoAccountRegistered(t *testing.T) {
	doc := `{"schemaVersion":2,"phase":"connections","installation":"2abc","complete":true,"connections":[]}`
	s := serverForConnectionsList(t, doc)
	res, _, err := s.handleListCloudConnections(context.Background(), nil, tools.EmptyInput{})
	if err != nil || isError(res) {
		t.Fatalf("%v %s", err, resultText(res))
	}
	got := strings.ToLower(resultText(res))
	if !strings.Contains(got, "no cloud account") {
		t.Errorf("an empty complete listing does not say none are registered: %s", resultText(res))
	}
}

// Below the version floor the call is refused at the gate, before any document
// is read, so an old formae never gets a chance to emit a document this build
// cannot parse anyway.
func TestListCloudConnections_RefusedBelowTheVersionFloor(t *testing.T) {
	// detectFn is package-level state: an earlier test's withFakeVersion left it
	// forced to return "0.0.0" (its t.Cleanup, not a restore to detectFromCLI),
	// so in a full-package run loginStub's own --version reply is never
	// consulted here. "0.0.0" is still below the floor either way.
	s := serverWithLoginBin(t, loginStub(t, "echo '{}'\n"))
	res, _, err := s.handleListCloudConnections(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("list_cloud_connections: %v", err)
	}
	if !isError(res) {
		t.Fatalf("a formae below the floor was accepted: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "0.89.0") {
		t.Errorf("the refusal does not name the required version: %s", resultText(res))
	}
}

// Trailing bytes after the document mean this is not a document this build can
// read, and taking the first value would be a guess. The result must be the
// exit-status fallback, not the code the trailing-content document happens to
// name: asserting on a wording property (e.g. "no mention of signing in") would
// pass by accident whenever the decoded message does not happen to use that
// word, without the EOF check ever running.
func TestRunConnect_RefusesTrailingContent(t *testing.T) {
	env := `{"schemaVersion":2,"code":"auth_failed"}`
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s}'\nexit 1\n", env)))
	res, _, _ := s.handleConnectCloudAccount(context.Background(), nil,
		tools.ConnectCloudAccountInput{Account: "123456789012"})
	got := resultText(res)
	if !strings.Contains(got, "formae connect failed (exit 1)") {
		t.Errorf("result does not carry the exit-status fallback: %s", got)
	}
	if strings.Contains(got, describeConnectFailure("auth_failed")) {
		t.Errorf("malformed trailing content was decoded as the auth_failed code: %s", got)
	}
}
