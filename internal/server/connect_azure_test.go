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
	// Tenant id is an authentication hint the CLI derives automatically when
	// absent, so it must not appear unless the caller supplied one. Client id
	// is never among this tool's arguments at all: the credential-less
	// register-only path is a terminal command for the user, never something
	// this tool drives.
	if strings.Contains(gotArgv, "--tenant-id") {
		t.Errorf("argv carries --tenant-id when the caller did not supply one: %s", gotArgv)
	}
	if strings.Contains(gotArgv, "--client-id") {
		t.Errorf("argv carries a register-only flag this tool must never pass: %s", gotArgv)
	}
}

// location, resource_group, and tenant_id all default on the CLI side, so the
// tool only forwards them when the caller actually supplied one.
func TestConnectAzureSubscription_PassesLocationAndResourceGroupWhenGiven(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, azureRegisteredJSON))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription:  testAzureSubscription,
		Location:      "westeurope",
		ResourceGroup: "my-rg",
		TenantID:      testAzureTenant,
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
	if !strings.Contains(got, "--tenant-id "+testAzureTenant) {
		t.Errorf("argv does not carry --tenant-id: %s", got)
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

// The CLI never spawns an `az login`. When there are no usable credentials it
// fails naming the exact command to run, and that command has to reach the
// caller: this is the only way a headless operator (or an agent driving this
// tool) can find out what to run.
//
// Drives the real handler with a real CLI failure document - not just
// decodeConnectFailure in isolation - because a fix that only works one level
// down and never reaches the tool's own result is the exact defect this
// vertical has already shipped twice.
const realAzureCredentialsRequiredFailure = `{"schemaVersion":1,"code":"credentials_required",` +
	`"message":"no usable Azure credentials for this subscription; run the sign-in and re-run this command",` +
	`"details":{"command":"az login --tenant 22222222-2222-2222-2222-222222222222"}}`

func TestConnectAzureSubscription_CarriesTheLoginCommand(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\nexit 1\n", realAzureCredentialsRequiredFailure)))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription: testAzureSubscription,
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if !isError(res) {
		t.Fatalf("a credential failure was reported as success: %s", resultText(res))
	}

	got := resultText(res)
	if !strings.Contains(got, "az login --tenant 22222222-2222-2222-2222-222222222222") {
		t.Errorf("the az login command did not reach the caller:\n%s", got)
	}
	// The description this code shares with GCP must not tell an Azure
	// operator to run gcloud: the code is the same across clouds, so the
	// static prose must be cloud-neutral, with the actual remedy carried
	// entirely by the relayed command.
	if strings.Contains(strings.ToLower(got), "gcloud") || strings.Contains(got, "Google") {
		t.Errorf("a GCP-specific remedy leaked into an Azure failure:\n%s", got)
	}
}

// orphaned_trust is the one failure where re-running does not simply retry a
// no-op: provisioning succeeded and registration did not, so the subscription
// already grants a near-owner identity to an installation the control plane
// does not know about. The surviving coordinates (resource group, identity,
// client id) exist specifically so an automated caller can find and finish
// registering that identity - the review that added them to the CLI required
// it. Drives the real handler with a real CLI failure document, the same
// shape the other relay tests in this file use, because a fix that lands on
// the CLI path and never reaches this tool's result is a defect this
// vertical has now shipped more than once.
const realAzureOrphanedTrustFailure = `{"schemaVersion":1,"code":"orphaned_trust",` +
	`"message":"the subscription now grants access to an installation the control plane does not know about; ` +
	`there is no rollback, and this identity holds near-owner access until it is registered. Re-run this command ` +
	`to finish: registration failed",` +
	`"details":{"resourceGroup":"formae-ai",` +
	`"identity":"/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/formae-ai/providers/` +
	`Microsoft.ManagedIdentity/userAssignedIdentities/formae-connect-2abc",` +
	`"clientId":"44444444-4444-4444-4444-444444444444"}}`

func TestConnectAzureSubscription_CarriesTheOrphanedTrustCoordinates(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\nexit 1\n", realAzureOrphanedTrustFailure)))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription: testAzureSubscription,
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if !isError(res) {
		t.Fatalf("an orphaned identity was reported as success: %s", resultText(res))
	}

	got := resultText(res)
	for _, want := range []string{
		"formae-ai",
		"/resourceGroups/formae-ai/providers/Microsoft.ManagedIdentity/userAssignedIdentities/formae-connect-2abc",
		"44444444-4444-4444-4444-444444444444",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the surviving coordinate %q did not reach the caller:\n%s", want, got)
		}
	}
}

// api_disabled is shared with GCP, which raises it for an unenabled Google
// API - the description this build showed for it named Google unconditionally,
// so an Azure operator whose resource provider is not registered was told to
// enable a Google API instead of being told which Azure provider to
// register. The remedy is one command and formae will not run it uninvited,
// so the provider name has to reach the caller.
const realAzureApiDisabledFailure = `{"schemaVersion":1,"code":"api_disabled",` +
	`"message":"the Microsoft.ContainerService resource provider is not registered on this subscription; register it and re-run",` +
	`"details":{"provider":"Microsoft.ContainerService"}}`

func TestConnectAzureSubscription_NamesTheUnregisteredProvider(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\nexit 1\n", realAzureApiDisabledFailure)))

	res, _, err := s.handleConnectAzureSubscription(context.Background(), nil, tools.ConnectAzureSubscriptionInput{
		Subscription: testAzureSubscription,
	})
	if err != nil {
		t.Fatalf("connect_azure_subscription: %v", err)
	}
	if !isError(res) {
		t.Fatalf("an unregistered provider was reported as success: %s", resultText(res))
	}

	got := resultText(res)
	if !strings.Contains(got, "Microsoft.ContainerService") {
		t.Errorf("the unregistered provider's name did not reach the caller:\n%s", got)
	}
	if strings.Contains(strings.ToLower(got), "google") {
		t.Errorf("a GCP-specific remedy leaked into an Azure failure:\n%s", got)
	}
}

// An MCP-native caller has no terminal and no reason to know a formae binary
// exists, so the credential-less path cannot end in a command for the user to
// run. Registration after a self-deployed ARM template is the harness's work:
// it already runs the binary for every other connect, and on a machine where
// that binary is not even on PATH a printed command is unfollowable advice.
func TestRegisterAzureTrust_DrivesTheRegisterOnlyPath(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, azureRegisteredJSON))

	res, _, err := s.handleRegisterAzureTrust(context.Background(), nil, tools.RegisterAzureTrustInput{
		Subscription: testAzureSubscription,
		TenantID:     testAzureTenant,
		ClientID:     testAzureClient,
	})
	if err != nil {
		t.Fatalf("register_azure_trust: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	// --client-id is the CLI's register-only signal and --tenant-id is required
	// alongside it; either one missing turns this into a provisioning attempt,
	// which is the one thing this path exists to avoid.
	gotArgv := readFile(t, argv)
	for _, want := range []string{
		"--subscription " + testAzureSubscription,
		"--tenant-id " + testAzureTenant,
		"--client-id " + testAzureClient,
	} {
		if !strings.Contains(gotArgv, want) {
			t.Errorf("argv does not carry %q: %s", want, gotArgv)
		}
	}

	got := resultText(res)
	for _, want := range []string{testAzureSubscription, testAzureTenant, testAzureClient} {
		if !strings.Contains(got, want) {
			t.Errorf("result does not carry %q:\n%s", want, got)
		}
	}
}

const azureTemplateJSON = `{"schemaVersion":2,"phase":"template","cloud":"azure",` +
	`"installation":"3IjUFLA75bBkM4DmUp2c6TgHeu9","formaeTenantId":"default",` +
	`"deepLink":"https://portal.azure.com/#create/Microsoft.Template/uri/https%3A%2F%2Fconsole.formae.ai%2Fazure%2Ftrust.json",` +
	`"templateUrl":"https://console.formae.ai/azure/trust.json?installation=3IjUFLA75bBkM4DmUp2c6TgHeu9",` +
	`"template":{"$schema":"https://schema.management.azure.com/schemas/2018-05-01/subscriptionDeploymentTemplate.json#"}}`

// The deep link is the credential-less path's whole value: it opens the portal
// with the template already fetched and its parameters filled in, so it asks
// nothing of the machine the user is talking to us from. It has to reach the
// caller as a link to show, exactly as AWS's quick-create URL does.
func TestGetAzureTrustTemplate_ShowsTheDeepLink(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, azureTemplateJSON))

	res, _, err := s.handleGetAzureTrustTemplate(context.Background(), nil, tools.GetAzureTrustTemplateInput{})
	if err != nil {
		t.Fatalf("get_azure_trust_template: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	got := resultText(res)
	if !strings.Contains(got, "portal.azure.com") {
		t.Errorf("result does not carry the deep link:\n%s", got)
	}
	// The follow-up is a tool call, never a command for the user to run: they
	// have no terminal in this flow and no reason to know the binary exists.
	if !strings.Contains(got, "register_azure_trust") {
		t.Errorf("result does not name the tool that finishes the flow:\n%s", got)
	}

	gotArgv := readFile(t, argv)
	for _, want := range []string{"connect", "azure", "template", "--output-consumer machine"} {
		if !strings.Contains(gotArgv, want) {
			t.Errorf("argv does not carry %q: %s", want, gotArgv)
		}
	}
}

// A document that is not the template phase means this build and the producer
// disagree about the protocol, which must not be rendered as if understood.
func TestGetAzureTrustTemplate_RejectsAnUnidentifiableDocument(t *testing.T) {
	s := serverWithLoginBin(t, argvStub(t, filepath.Join(t.TempDir(), "argv"), azureRegisteredJSON))

	res, _, err := s.handleGetAzureTrustTemplate(context.Background(), nil, tools.GetAzureTrustTemplateInput{})
	if err != nil {
		t.Fatalf("get_azure_trust_template: %v", err)
	}
	if !isError(res) {
		t.Errorf("a registered document was rendered as a template: %s", resultText(res))
	}
}
