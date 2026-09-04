package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// Azure gets one tool, the same shape as GCP: there is one interactive path,
// so there is nothing to choose between. formae obtains the operator's
// ambient Azure credentials, provisions the managed identity, and registers
// it in a single invocation.
//
// Unlike GCP there is no --allow-login equivalent: the CLI never spawns a
// sign-in for Azure. When there are no usable credentials it fails naming the
// exact `az login` command to run, which is why the credential-required
// relay in connect.go matters here more than anywhere else in this file.
//
// The credential-less register-only path (an operator who deploys the ARM
// template themselves and supplies the resulting tenant and client id) is
// deliberately not an argument this tool accepts: it is a terminal command
// for the user to run in their own session, never something this tool
// drives.
func (s *Server) handleConnectAzureSubscription(ctx context.Context, _ *mcp.CallToolRequest,
	input tools.ConnectAzureSubscriptionInput) (*mcp.CallToolResult, any, error) {
	args := []string{
		"connect", "azure", "--subscription", input.Subscription,
		"--no-input", "--output-consumer", "machine", "--output-schema", "json",
	}
	// Location, resource group, and tenant id all default on the CLI side, so
	// they are forwarded only when the caller actually supplied one.
	if input.Location != "" {
		args = append(args, "--location", input.Location)
	}
	if input.ResourceGroup != "" {
		args = append(args, "--resource-group", input.ResourceGroup)
	}
	if input.TenantID != "" {
		args = append(args, "--tenant-id", input.TenantID)
	}

	out, err := s.runConnect(ctx, args)
	if err != nil {
		return errorResult(err), nil, nil
	}

	doc, err := decodeRegisteredDoc(out)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return textResult(renderAzureRegistered(doc)), nil, nil
}

// renderAzureRegistered reports what the registration did, naming the two
// coordinates Azure actually carries: it has no role, so renderRegistered's
// wording cannot be reused as-is.
func renderAzureRegistered(d registeredDoc) string {
	return renderRegisteredDoc(d, "subscription", "the same identity",
		"Tenant: "+d.AzureTenantID, "Client id: "+d.AzureClientID)
}

// handleRegisterAzureTrust drives the CLI's register-only path, where trust
// already exists because the operator deployed the template themselves.
//
// The harness runs this the same way it runs every other connect. An MCP-native
// caller has no terminal in the loop and no reason to know a formae binary is
// on the machine -- and frequently it is not on PATH at all -- so a printed
// command was never a path they could follow.
func (s *Server) handleRegisterAzureTrust(ctx context.Context, _ *mcp.CallToolRequest,
	input tools.RegisterAzureTrustInput) (*mcp.CallToolResult, any, error) {
	// --client-id is the CLI's register-only signal, and it requires
	// --tenant-id alongside it. Both are required inputs here so the pairing
	// cannot be got wrong: a client id arriving alone would be read as a
	// provisioning run against ambient credentials.
	args := []string{
		"connect", "azure", "--subscription", input.Subscription,
		"--tenant-id", input.TenantID, "--client-id", input.ClientID,
		"--no-input", "--output-consumer", "machine", "--output-schema", "json",
	}

	out, err := s.runConnect(ctx, args)
	if err != nil {
		return errorResult(err), nil, nil
	}

	doc, err := decodeRegisteredDoc(out)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return textResult(renderAzureRegistered(doc)), nil, nil
}

// azureTemplateDoc is the CLI's template emit. Only the link and the
// coordinates are read here: the embedded template is for a caller writing it
// to a file or a pipeline, which is not what this tool is for.
type azureTemplateDoc struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Phase          string `json:"phase"`
	Cloud          string `json:"cloud"`
	Installation   string `json:"installation"`
	FormaeTenantID string `json:"formaeTenantId"`
	DeepLink       string `json:"deepLink"`
	TemplateURL    string `json:"templateUrl"`
}

// handleGetAzureTrustTemplate returns the portal link for the credential-less
// path.
//
// This exists because the link used to appear only in the CLI's human-readable
// stderr, so the one path that needs nothing of the user's machine was the one
// path an MCP caller could not offer. It asks the producer for the link the
// same way the AWS quick-create flow does, rather than rebuilding the URL here
// where it would drift from the CLI's.
func (s *Server) handleGetAzureTrustTemplate(ctx context.Context, _ *mcp.CallToolRequest,
	_ tools.GetAzureTrustTemplateInput) (*mcp.CallToolResult, any, error) {
	out, err := s.runConnect(ctx, []string{
		"connect", "azure", "template",
		"--output-consumer", "machine", "--output-schema", "json",
	})
	if err != nil {
		return errorResult(err), nil, nil
	}

	var doc azureTemplateDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return errorResult(errUnreadableConnect), nil, nil
	}
	if doc.SchemaVersion != connectSchemaVersion || doc.Phase != "template" {
		return errorResult(errUnreadableConnect), nil, nil
	}

	return textResult(renderAzureTemplate(doc)), nil, nil
}

// renderAzureTemplate tells the caller what to put in front of the user, and
// what to do with what comes back.
func renderAzureTemplate(d azureTemplateDoc) string {
	var b strings.Builder
	b.WriteString("Show the user this link and wait for them to deploy it:\n\n  ")
	b.WriteString(d.DeepLink)
	b.WriteString("\n\nIt opens the Azure portal with the trust template already loaded and this " +
		"installation's coordinates filled in, so there is nothing to paste and no CLI to install. " +
		"They pick a region; everything else is prefilled. It creates a managed identity with " +
		"Contributor and User Access Administrator on the subscription they deploy it into.\n\n")
	fmt.Fprintf(&b, "Installation: %s\nformae tenant: %s\n\n", d.Installation, d.FormaeTenantID)
	b.WriteString("When they say the deployment finished, read tenantId and clientId from its " +
		"Outputs tab and call register_azure_trust with them, together with the subscription id. " +
		"Do not ask them to run a formae command.")
	return b.String()
}
