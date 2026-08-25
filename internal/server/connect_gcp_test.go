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
	testGcpProject  = "example-project"
	testGcpProvider = "//iam.googleapis.com/projects/123456789012/locations/global/" +
		"workloadIdentityPools/formae-ai/providers/formae-ai"

	gcpRegisteredJSON = `{"schemaVersion":2,"phase":"registered","status":"registered_unverified",` +
		`"cloud":"gcp","account":"` + testGcpProject + `","workloadIdentityProvider":"` + testGcpProvider + `"}`

	gcpAlreadyJSON = `{"schemaVersion":2,"phase":"registered","status":"already_registered",` +
		`"cloud":"gcp","account":"` + testGcpProject + `","workloadIdentityProvider":"` + testGcpProvider + `"}`
)

// The whole GCP flow is one call: provision and register together, because
// there is no console step to split it around.
func TestConnectGcpProject_ReportsTheProjectAndProvider(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, gcpRegisteredJSON))

	res, _, err := s.handleConnectGcpProject(context.Background(), nil, tools.ConnectGcpProjectInput{
		Project: testGcpProject,
	})
	if err != nil {
		t.Fatalf("connect_gcp_project: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}

	got := resultText(res)
	if !strings.Contains(got, testGcpProject) {
		t.Errorf("result does not name the project:\n%s", got)
	}
	if !strings.Contains(got, testGcpProvider) {
		t.Errorf("result does not name the workload identity provider:\n%s", got)
	}
	// A GCP connection has no role, and renderRegistered would have printed an
	// empty one. Nothing here may invite the reader to believe in a value that
	// never existed.
	if strings.Contains(got, "Role:") {
		t.Errorf("a GCP result printed a role line:\n%s", got)
	}

	gotArgv := readFile(t, argv)
	if !strings.Contains(gotArgv, "--project") {
		t.Errorf("argv does not carry --project: %s", gotArgv)
	}
	if strings.Contains(gotArgv, "--workload-identity-provider") {
		t.Errorf("the default path passed --workload-identity-provider, so it registered "+
			"instead of provisioning: %s", gotArgv)
	}
}

// Supplying the provider means the operator stood the federation up
// themselves, which is the one case where formae has nothing to provision.
func TestConnectGcpProject_RegisterOnlyWhenTheProviderIsGiven(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	s := serverWithLoginBin(t, argvStub(t, argv, gcpRegisteredJSON))

	res, _, err := s.handleConnectGcpProject(context.Background(), nil, tools.ConnectGcpProjectInput{
		Project:                  testGcpProject,
		WorkloadIdentityProvider: testGcpProvider,
	})
	if err != nil {
		t.Fatalf("connect_gcp_project: %v", err)
	}
	if isError(res) {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}
	if got := readFile(t, argv); !strings.Contains(got, "--workload-identity-provider") {
		t.Errorf("argv does not carry --workload-identity-provider, so this was not the register-only path: %s", got)
	}
}

// already_registered is the idempotent case, exactly as on AWS: the row
// already names this same federation. Reporting it as a failure would make
// re-running the journey look broken when it is what makes re-running safe.
func TestConnectGcpProject_TreatsAlreadyRegisteredAsSuccess(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", gcpAlreadyJSON)))

	res, _, err := s.handleConnectGcpProject(context.Background(), nil, tools.ConnectGcpProjectInput{
		Project: testGcpProject,
	})
	if err != nil {
		t.Fatalf("connect_gcp_project: %v", err)
	}
	if isError(res) {
		t.Fatalf("already_registered was reported as an error: %s", resultText(res))
	}
	if got := resultText(res); !strings.Contains(got, "already connected") {
		t.Errorf("result does not say the project was already connected:\n%s", got)
	}
}

// The CLI's register-only warning states what was not checked. It has to reach
// the user, or "registered" reads as "working".
func TestConnectGcpProject_PassesTheUnverifiedWarningThrough(t *testing.T) {
	warned := `{"schemaVersion":2,"phase":"registered","status":"registered_unverified",` +
		`"cloud":"gcp","account":"` + testGcpProject + `","workloadIdentityProvider":"` + testGcpProvider + `",` +
		`"warnings":["the coordinate was validated for shape only"]}`
	s := serverWithLoginBin(t, loginStub(t, fmt.Sprintf("echo '%s'\n", warned)))

	res, _, err := s.handleConnectGcpProject(context.Background(), nil, tools.ConnectGcpProjectInput{
		Project:                  testGcpProject,
		WorkloadIdentityProvider: testGcpProvider,
	})
	if err != nil {
		t.Fatalf("connect_gcp_project: %v", err)
	}
	if got := resultText(res); !strings.Contains(got, "shape only") {
		t.Errorf("the producer's warning did not reach the user:\n%s", got)
	}
}

// A document this build cannot identify is refused rather than rendered as a
// zero value, the same rule the AWS documents follow.
func TestConnectGcpProject_RefusesADocumentItCannotIdentify(t *testing.T) {
	s := serverWithLoginBin(t, loginStub(t, "echo '{\"schemaVersion\":99,\"phase\":\"registered\"}'\n"))

	res, _, err := s.handleConnectGcpProject(context.Background(), nil, tools.ConnectGcpProjectInput{
		Project: testGcpProject,
	})
	if err != nil {
		t.Fatalf("connect_gcp_project: %v", err)
	}
	if !isError(res) {
		t.Errorf("an unidentifiable document was rendered as success: %s", resultText(res))
	}
}
