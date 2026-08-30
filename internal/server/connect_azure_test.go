package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

const (
	testAzureSubscription = "11111111-1111-1111-1111-111111111111"
	testAzureTenant       = "22222222-2222-2222-2222-222222222222"
	testAzureClient       = "33333333-3333-3333-3333-333333333333"

	azureRegisteredJSON = `{"schemaVersion":2,"phase":"registered","status":"registered_unverified",` +
		`"cloud":"azure","account":"` + testAzureSubscription + `","azureTenantId":"` + testAzureTenant +
		`","azureClientId":"` + testAzureClient + `"}`

	azureAlreadyJSON = `{"schemaVersion":2,"phase":"registered","status":"already_registered",` +
		`"cloud":"azure","account":"` + testAzureSubscription + `","azureTenantId":"` + testAzureTenant +
		`","azureClientId":"` + testAzureClient + `"}`
)

// Azure has one interactive path, like GCP: formae obtains ambient
// credentials, provisions the connection, and registers it in a single call.
// There is no link-then-register pair to choose between.
func TestConnectAzureSubscription_ReportsTheSubscriptionTenantAndClient(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, azureRegisteredJSON))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription: testAzureSubscription,
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	got := resultText(res)
	for _, want := range []string{testAzureSubscription, testAzureTenant, testAzureClient} {
		if !strings.Contains(got, want) {
			t.Errorf("result does not carry %q:\n%s", want, got)
		}
	}
	// An Azure connection has no role, and renderRegistered would have printed
	// an empty one. Nothing here may invite the reader to believe in a value
	// that never existed.
	if strings.Contains(got, "Role:") {
		t.Errorf("an Azure result printed a role line:\n%s", got)
	}

	gotArgv := readFile(t, argv)
	if !strings.Contains(gotArgv, "--subscription "+testAzureSubscription) {
		t.Errorf("argv does not carry --subscription: %s", gotArgv)
	}
	if !strings.Contains(gotArgv, "--no-input") {
		t.Errorf("argv does not disable prompts, so the CLI could block on one: %s", gotArgv)
	}
	if strings.Contains(gotArgv, "--location") || strings.Contains(gotArgv, "--resource-group") {
		t.Errorf("argv carries a default-only flag the caller did not ask for: %s", gotArgv)
	}
	// There is no register-only path through this tool: supplying a tenant or
	// client id is not among its arguments, and the credential-less path is a
	// terminal command for the user, never something this tool drives.
	if strings.Contains(gotArgv, "--tenant-id") || strings.Contains(gotArgv, "--client-id") {
		t.Errorf("argv carries a register-only flag this tool must never pass: %s", gotArgv)
	}
}

// location and resource_group both default on the CLI side, so the tool only
// forwards them when the caller actually supplied one.
func TestConnectAzureSubscription_PassesLocationAndResourceGroupWhenGiven(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, azureRegisteredJSON))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription:  testAzureSubscription,
		Location:      "westeurope",
		ResourceGroup: "my-rg",
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	got := readFile(t, argv)
	if !strings.Contains(got, "--location westeurope") {
		t.Errorf("argv does not carry --location: %s", got)
	}
	if !strings.Contains(got, "--resource-group my-rg") {
		t.Errorf("argv does not carry --resource-group: %s", got)
	}
}

// already_registered is the idempotent case, exactly as on AWS and GCP: the
// row already names this same identity.
func TestConnectAzureSubscription_TreatsAlreadyRegisteredAsSuccess(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", azureAlreadyJSON)))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription: testAzureSubscription,
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if isError(res) {
		t.Fatalf("already_registered was reported as an error: %s", resultText(res))
	}
	if got := resultText(res); !strings.Contains(got, "already connected") {
		t.Errorf("result does not say the subscription was already connected:\n%s", got)
	}
}

// A document this build cannot identify is refused rather than rendered as a
// zero value, the same rule the AWS and GCP documents follow.
func TestConnectAzureSubscription_RefusesADocumentItCannotIdentify(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, "echo '{\"schemaVersion\":99,\"phase\":\"registered\"}'\n"))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription: testAzureSubscription,
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if !isError(res) {
		t.Errorf("an unidentifiable document was rendered as success: %s", resultText(res))
	}
}
