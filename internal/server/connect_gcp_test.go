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
	// The sign-in has to be reachable from here. Without the opt-in, machine
	// output is read as "nobody is present" and the operator is handed a
	// command to run by hand instead.
	if !strings.Contains(gotArgv, "--allow-login") {
		t.Errorf("argv does not carry --allow-login, so a missing credential would "+
			"be reported instead of signed in: %s", gotArgv)
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

// TestGcpFailureCodesCarryARemedy guards the map that translates producer
// codes for the user.
//
// It exists because the integration test found the gap the hard way: the CLI
// grew four GCP codes, this map did not, and a run with no credentials
// reported "formae connect failed (credentials_required)" — a refusal with no
// remedy, when the remedy is one command the producer already names.
func TestGcpFailureCodesCarryARemedy(t *testing.T) {
	for _, code := range []string{"credentials_required", "gcloud_missing", "project_unreachable", "api_disabled"} {
		desc, ok := connectFailureDescriptions[code]
		if !ok {
			t.Errorf("%s has no description, so the user sees a bare code", code)
			continue
		}
		if desc == "" {
			t.Errorf("%s has an empty description", code)
		}
	}
	// The one whose remedy is a command has to name it: that is the whole
	// point of describing it rather than passing the code through.
	if got := connectFailureDescriptions["credentials_required"]; !strings.Contains(got, "gcloud auth application-default login") {
		t.Errorf("credentials_required does not name the command to run: %q", got)
	}
}

// A sign-in that could not finish must reach the caller with the URL gcloud
// printed, because that is the only way to complete it.
//
// The producer's own message is withheld deliberately - it is built from
// plugin error strings and a Pkl failure quotes profile source, which for a
// classic profile can hold an inline password. Details are different: each is
// a value the producer chose under a key it declared, so a named key from a
// named code is safe to relay where the whole message is not.
//
// Without this the headless case is a dead end. formae runs gcloud, gcloud
// cannot open a browser, so it prints a URL and waits for a code on a stdin
// that is /dev/null; the run fails having printed the one thing that would
// have let the operator finish, and this build then replaced it with a canned
// line telling them to run the command they cannot complete either.
func TestCredentialsRequiredCarriesTheSignInURL(t *testing.T) {
	const url = "https://accounts.google.com/o/oauth2/auth?response_type=code&client_id=764086051850-x.apps.googleusercontent.com"
	envelope := `{"schemaVersion":1,"code":"credentials_required",` +
		`"message":"the Google Cloud sign-in did not complete; what gcloud reported is in the details",` +
		`"details":{"command":"gcloud auth application-default login",` +
		`"output":"Go to the following link in your browser:\n\n    ` + url + `\n"}}`

	err := decodeConnectFailure([]byte(envelope), 1)
	if err == nil {
		t.Fatal("a failure envelope must produce an error")
	}
	got := err.Error()

	if !strings.Contains(got, url) {
		t.Errorf("the sign-in URL did not reach the caller:\n%s", got)
	}
	if !strings.Contains(got, "gcloud auth application-default login") {
		t.Errorf("the remedy command was lost:\n%s", got)
	}
	// The producer's message stays withheld: relaying details is not a licence
	// to relay prose.
	if strings.Contains(got, "what gcloud reported is in the details") {
		t.Errorf("the producer's message crossed, which it must never do:\n%s", got)
	}
}

// Details are relayed by an allowlist, not wholesale. A code that declares no
// safe keys carries none, so a producer that starts attaching something
// sensitive to an unrelated failure cannot leak it through this path.
func TestUnlistedDetailsAreNotRelayed(t *testing.T) {
	envelope := `{"schemaVersion":1,"code":"auth_failed",` +
		`"message":"whatever",` +
		`"details":{"output":"tokens and other things nobody asked to see"}}`

	got := decodeConnectFailure([]byte(envelope), 1).Error()
	if strings.Contains(got, "nobody asked to see") {
		t.Errorf("details crossed for a code that declares none:\n%s", got)
	}
	if got != connectFailureDescriptions["auth_failed"] {
		t.Errorf("description changed: %q", got)
	}
}

// An envelope with no details at all still reads as it always did, so the
// common failure is not padded with an empty section.
func TestFailureWithoutDetailsIsUnchanged(t *testing.T) {
	envelope := `{"schemaVersion":1,"code":"credentials_required","message":"x"}`
	got := decodeConnectFailure([]byte(envelope), 1).Error()
	if got != connectFailureDescriptions["credentials_required"] {
		t.Errorf("a detail-less failure changed shape: %q", got)
	}
}
