package server

import (
	"context"
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
	// Location and resource group both default on the CLI side, so they are
	// forwarded only when the caller actually supplied one.
	if input.Location != "" {
		args = append(args, "--location", input.Location)
	}
	if input.ResourceGroup != "" {
		args = append(args, "--resource-group", input.ResourceGroup)
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

// renderAzureRegistered reports what the registration did, naming the
// coordinate Azure actually carries.
//
// renderRegistered cannot be reused: it prints "Role: <arn>", and an Azure
// document has no role. Printing an empty one would invite the reader to
// believe in a value that never existed.
func renderAzureRegistered(d registeredDoc) string {
	var b strings.Builder
	if d.Status == statusAlreadyRegistered {
		fmt.Fprintf(&b, "Subscription %s was already connected to this installation with the same identity.\n", d.Account)
	} else {
		fmt.Fprintf(&b, "Connected subscription %s.\n", d.Account)
	}
	fmt.Fprintf(&b, "Tenant: %s\n", d.AzureTenantID)
	fmt.Fprintf(&b, "Client id: %s\n", d.AzureClientID)
	for _, w := range d.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s\n", w)
	}
	return b.String()
}
