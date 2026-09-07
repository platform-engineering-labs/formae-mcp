package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// Two more ways to connect a cloud account, both driving `formae connect aws`
// with the user's own local credentials rather than a console link:
//
// `list_aws_profiles` shows the local AWS profiles and the account each one
// resolves to, so picking a profile is an informed choice about where trust
// gets provisioned. `provision_cloud_role` then creates the role with those
// credentials and registers it in one run — no console step, no user-applied
// stack, which is what makes it the destructive one of the two.

// serverForAwsProfiles builds a server whose formae binary emits doc and whose
// detected version clears the FeatureCloudConnectionList floor these tools
// share with list_cloud_connections.
func serverForAwsProfiles(t *testing.T, doc string) *Server {
	t.Helper()
	withFakeVersion(t, "0.89.0-dev.9")
	return serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", doc)))
}

const awsProfilesJSON = `{"schemaVersion":2,"phase":"awsProfiles",` +
	`"profiles":[{"name":"blue-admin","account":"123456789012"},` +
	`{"name":"sandbox","unavailable":"the SSO session has expired"}],"warnings":[]}`

// Each profile's account or reason must land on the line naming that profile,
// not merely appear somewhere in the output: a test that only checks
// substring presence would still pass if the two profiles' details were
// swapped.
func TestListAwsProfiles_RendersEachProfileWithItsOwnAccountOrReason(t *testing.T) {
	s := serverForAwsProfiles(t, awsProfilesJSON)

	res, _, err := s.handleListAwsProfiles(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("list_aws_profiles: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	lines := strings.Split(resultText(res), "\n")
	wantLines := []string{
		"blue-admin: account 123456789012",
		"sandbox: unavailable (the SSO session has expired)",
	}
	for _, want := range wantLines {
		found := false
		for _, line := range lines {
			if strings.TrimSpace(line) == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no line reads exactly %q; got:\n%s", want, resultText(res))
		}
	}
}

// An unavailable profile is not an error: the SSO-expired case is something
// the user can act on, so the call succeeds and names the reason.
func TestListAwsProfiles_AnUnavailableProfileIsNotAnError(t *testing.T) {
	doc := `{"schemaVersion":2,"phase":"awsProfiles",` +
		`"profiles":[{"name":"sandbox","unavailable":"the SSO session has expired"}],"warnings":[]}`
	s := serverForAwsProfiles(t, doc)

	res, _, err := s.handleListAwsProfiles(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("list_aws_profiles: %v", err)
	}
	if isError(res) {
		t.Fatalf("an unavailable profile was reported as a tool error: %s", resultText(res))
	}
}

// A completely empty list is the normal "quick-create is the only path" case,
// not a failure.
func TestListAwsProfiles_ReportsAnEmptyListAsNormal(t *testing.T) {
	doc := `{"schemaVersion":2,"phase":"awsProfiles","profiles":[],"warnings":[]}`
	s := serverForAwsProfiles(t, doc)

	res, _, err := s.handleListAwsProfiles(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("list_aws_profiles: %v", err)
	}
	if isError(res) {
		t.Fatalf("an empty profile list was reported as an error: %s", resultText(res))
	}
}

// A document missing both discriminators decodes cleanly into a zero value
// under plain json.Unmarshal, so the handler must validate rather than trust
// it.
func TestListAwsProfiles_RefusesADocumentItCannotIdentify(t *testing.T) {
	s := serverForAwsProfiles(t, `{"profiles":[]}`)

	res, _, _ := s.handleListAwsProfiles(context.Background(), nil, tools.EmptyInput{})
	if !isError(res) {
		t.Errorf("an unidentifiable document was accepted: %s", resultText(res))
	}
}

// Below the shared version floor the call is refused at the gate before any
// document is read.
func TestListAwsProfiles_RefusedBelowTheVersionFloor(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, "echo '{}'\n"))

	res, _, err := s.handleListAwsProfiles(context.Background(), nil, tools.EmptyInput{})
	if err != nil {
		t.Fatalf("list_aws_profiles: %v", err)
	}
	if !isError(res) {
		t.Fatalf("a formae below the floor was accepted: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "0.89.0") {
		t.Errorf("the refusal does not name the required version: %s", resultText(res))
	}
}

const provisionedJSON = `{"schemaVersion":2,"phase":"registered","status":"registered_unverified",` +
	`"cloud":"aws","account":"123456789012","roleArn":"arn:aws:iam::123456789012:role/formae-connect-2abc"}`

// provision_cloud_role passes the account and the chosen local profile through
// to the CLI verbatim, in one invocation that both creates the role and
// registers it — no separate register_cloud_role call.
func TestProvisionCloudRole_PassesAccountAndProfileFlags(t *testing.T) {
	withFakeVersion(t, "0.89.0-dev.9")
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, provisionedJSON))

	res, _, err := s.handleProvisionCloudRole(context.Background(), nil, tools.ProvisionCloudRoleInput{
		Account:    "123456789012",
		AwsProfile: "blue-admin",
	})
	if err != nil {
		t.Fatalf("provision_cloud_role: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	got := readFile(t, argv)
	for _, want := range []string{"--account 123456789012", "--profile-aws blue-admin", "--no-input"} {
		if !strings.Contains(got, want) {
			t.Errorf("argv does not carry %q: %s", want, got)
		}
	}
	if strings.Contains(got, "--role-arn") {
		t.Errorf("argv carries --role-arn, which would make this the register-only path: %s", got)
	}
}

// The returned document is the same registered shape register_cloud_role
// already renders, so the account and role reach the caller identically.
func TestProvisionCloudRole_ReportsTheRegisteredAccountAndRole(t *testing.T) {
	withFakeVersion(t, "0.89.0-dev.9")
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", provisionedJSON)))

	res, _, err := s.handleProvisionCloudRole(context.Background(), nil, tools.ProvisionCloudRoleInput{
		Account:    "123456789012",
		AwsProfile: "blue-admin",
	})
	if err != nil {
		t.Fatalf("provision_cloud_role: %v", err)
	}
	got := resultText(res)
	for _, want := range []string{
		"123456789012",
		"arn:aws:iam::123456789012:role/formae-connect-2abc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result does not carry %q: %s", want, got)
		}
	}
}

// already_registered is success here too, for the same reason it is on
// register_cloud_role: it is what makes re-running the flow harmless.
func TestProvisionCloudRole_TreatsAlreadyRegisteredAsSuccess(t *testing.T) {
	withFakeVersion(t, "0.89.0-dev.9")
	doc := `{"schemaVersion":2,"phase":"registered","status":"already_registered",` +
		`"cloud":"aws","account":"123456789012","roleArn":"arn:aws:iam::123456789012:role/formae-connect-2abc"}`
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", doc)))

	res, _, err := s.handleProvisionCloudRole(context.Background(), nil, tools.ProvisionCloudRoleInput{
		Account:    "123456789012",
		AwsProfile: "blue-admin",
	})
	if err != nil {
		t.Fatalf("provision_cloud_role: %v", err)
	}
	if isError(res) {
		t.Fatalf("already_registered reported as an error: %s", resultText(res))
	}
}

// Below the shared version floor the call is refused before any subprocess
// argument would matter.
func TestProvisionCloudRole_RefusedBelowTheVersionFloor(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, "echo '{}'\n"))

	res, _, err := s.handleProvisionCloudRole(context.Background(), nil, tools.ProvisionCloudRoleInput{
		Account:    "123456789012",
		AwsProfile: "blue-admin",
	})
	if err != nil {
		t.Fatalf("provision_cloud_role: %v", err)
	}
	if !isError(res) {
		t.Fatalf("a formae below the floor was accepted: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "0.89.0") {
		t.Errorf("the refusal does not name the required version: %s", resultText(res))
	}
}

// provision_cloud_role creates infrastructure immediately with no user-applied
// step, unlike connect_cloud_account; it must carry DestructiveHint so a
// caller does not treat it as a plan. list_aws_profiles only reads local
// state, so it carries ReadOnlyHint.
func TestConnectTools_AnnotationsMatchWhatEachToolDoes(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	result, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	var listProfiles, provision *mcp.Tool
	for _, tool := range result.Tools {
		switch tool.Name {
		case "list_aws_profiles":
			listProfiles = tool
		case "provision_cloud_role":
			provision = tool
		}
	}
	if listProfiles == nil {
		t.Fatal("list_aws_profiles tool not registered")
	}
	if provision == nil {
		t.Fatal("provision_cloud_role tool not registered")
	}

	if listProfiles.Annotations == nil || !listProfiles.Annotations.ReadOnlyHint {
		t.Errorf("list_aws_profiles is not marked ReadOnlyHint: %+v", listProfiles.Annotations)
	}
	if provision.Annotations == nil || provision.Annotations.DestructiveHint == nil || !*provision.Annotations.DestructiveHint {
		t.Errorf("provision_cloud_role is not marked DestructiveHint: %+v", provision.Annotations)
	}
}
