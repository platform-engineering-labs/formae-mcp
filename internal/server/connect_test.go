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
