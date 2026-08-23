package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// This is the second, faster way to connect a cloud account, alongside the
// console-link flow in connect.go. When the user has local AWS credentials,
// formae can create the connect role directly with them instead of the user
// applying a CloudFormation stack by hand.
//
// list_aws_profiles shows the local profiles and the account each resolves
// to, so picking one is an informed choice about where trust gets
// provisioned. provision_cloud_role then creates the role with those
// credentials and registers it, in a single formae invocation — which is why,
// unlike connect_cloud_account, it is genuinely destructive: there is no
// console step standing between the tool call and the mutation.

// awsProfilesPhase is the phase value an AWS profiles listing carries.
const awsProfilesPhase = "awsProfiles"

// awsProfile is one local AWS profile. It carries either Account (it
// resolved) or Unavailable (it did not, with the reason) — never both, never
// neither. Both are omitempty on the producer side, so an unresolved profile
// has no "account" key at all.
type awsProfile struct {
	Name        string `json:"name"`
	Account     string `json:"account,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
}

// awsProfilesDoc is what `formae connect aws profiles` reports.
type awsProfilesDoc struct {
	SchemaVersion int          `json:"schemaVersion"`
	Phase         string       `json:"phase"`
	Profiles      []awsProfile `json:"profiles"`
	Warnings      []string     `json:"warnings"`
}

// errUnreadableAwsProfiles is an AWS profiles document this build could not
// read.
var errUnreadableAwsProfiles = errors.New("formae connect aws profiles produced output this build could not read; " +
	"the connected formae may be older than this plugin")

// decodeAwsProfilesDoc validates an AWS profiles document rather than
// trusting json.Unmarshal: a document missing both discriminators decodes
// cleanly into a zero value, and rendering that zero value would silently
// claim an empty listing.
func decodeAwsProfilesDoc(out []byte) (awsProfilesDoc, error) {
	var d awsProfilesDoc
	dec := json.NewDecoder(bytes.NewReader(out))
	if err := dec.Decode(&d); err != nil {
		return awsProfilesDoc{}, errUnreadableAwsProfiles
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return awsProfilesDoc{}, errUnreadableAwsProfiles
	}
	if d.SchemaVersion != connectSchemaVersion || d.Phase != awsProfilesPhase {
		return awsProfilesDoc{}, errUnreadableAwsProfiles
	}
	return d, nil
}

// handleListAwsProfiles reports the user's local AWS profiles and the account
// each one resolves to, so a caller can offer provision_cloud_role as an
// informed choice.
func (s *Server) handleListAwsProfiles(ctx context.Context, _ *mcp.CallToolRequest, _ tools.EmptyInput) (*mcp.CallToolResult, any, error) {
	bin := s.formaeBin()
	if err := featuregate.GuardFeatureContext(ctx, featuregate.FeatureCloudConnectionList, bin); err != nil {
		return errorResult(err), nil, nil
	}

	out, err := s.runConnect(ctx, []string{"connect", "aws", "profiles", "--output-consumer", "machine", "--output-schema", "json"})
	if err != nil {
		return errorResult(err), nil, nil
	}

	doc, err := decodeAwsProfilesDoc(out)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return textResult(renderAwsProfiles(doc)), nil, nil
}

// renderAwsProfiles tells the caller what each profile is and, for one that
// did not resolve, why — never dropping it, since the reason is something the
// user can act on.
func renderAwsProfiles(d awsProfilesDoc) string {
	var b strings.Builder
	if len(d.Profiles) == 0 {
		b.WriteString("No local AWS profiles were found. connect_cloud_account is the only path available.\n")
	}
	for _, p := range d.Profiles {
		if p.Account != "" {
			fmt.Fprintf(&b, "%s: account %s\n", p.Name, p.Account)
		} else {
			fmt.Fprintf(&b, "%s: unavailable (%s)\n", p.Name, p.Unavailable)
		}
	}
	for _, w := range d.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s\n", w)
	}
	return b.String()
}

// handleProvisionCloudRole creates the connect role with the named local AWS
// profile's credentials and registers it, in one invocation.
//
// This mutates immediately: unlike connect_cloud_account, there is no console
// step between this call and the role (and possibly the account-global OIDC
// provider) being created.
func (s *Server) handleProvisionCloudRole(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProvisionCloudRoleInput) (*mcp.CallToolResult, any, error) {
	bin := s.formaeBin()
	if err := featuregate.GuardFeatureContext(ctx, featuregate.FeatureCloudConnectionList, bin); err != nil {
		return errorResult(err), nil, nil
	}

	out, err := s.runConnect(ctx, []string{
		"connect", "aws", "--account", input.Account, "--profile-aws", input.AwsProfile,
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
