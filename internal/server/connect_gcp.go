package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
)

// GCP gets one tool where AWS has three.
//
// That asymmetry is the cloud's, not an inconsistency here. The AWS flow is
// link-then-register because CloudFormation's quick-create URL lets the
// operator apply a stack in their own console and come back with a role. GCP
// retired its only console-deployable template service, so there is no link to
// hand anyone: formae provisions with the operator's own credentials, and
// provisioning and registering are one invocation. Splitting it into two tools
// would invent a step that does not exist.
func (s *Server) handleConnectGcpProject(ctx context.Context, _ *mcp.CallToolRequest,
	input tools.ConnectGcpProjectInput) (*mcp.CallToolResult, any, error) {
	args := []string{
		"connect", "gcp", "--project", input.Project,
		"--no-input", "--output-consumer", "machine", "--output-schema", "json",
		// The operator is sitting in front of this server: it runs on their
		// own machine, started by their own agent session. So it opts into the
		// Google sign-in, which opens a browser they can complete.
		//
		// --no-input still applies and still means what it says: no terminal
		// prompt this server would have to answer. The two are separate
		// questions, and without saying so explicitly the sign-in was
		// unreachable from here and every operator was handed a command to run
		// by hand.
		"--allow-login",
	}
	// Register-only exists for an operator who provisioned federation
	// themselves and will not hand a CLI provisioning rights. It validates the
	// coordinate's shape and nothing else, and the CLI says so in a warning
	// that rides the document below.
	if input.WorkloadIdentityProvider != "" {
		args = append(args, "--workload-identity-provider", input.WorkloadIdentityProvider)
	}

	out, err := s.runConnect(ctx, args)
	if err != nil {
		return errorResult(err), nil, nil
	}

	doc, err := decodeRegisteredDoc(out)
	if err != nil {
		return errorResult(err), nil, nil
	}

	return textResult(renderGcpRegistered(doc)), nil, nil
}

// renderGcpRegistered reports what the registration did, naming the coordinate
// GCP actually carries.
//
// renderRegistered cannot be reused: it prints "Role: <arn>", and a GCP
// document has no role. Printing an empty one would invite the reader to
// believe in a value that never existed.
func renderGcpRegistered(d registeredDoc) string {
	var b strings.Builder
	if d.Status == statusAlreadyRegistered {
		fmt.Fprintf(&b, "Project %s was already connected to this installation with the same federation.\n", d.Account)
	} else {
		fmt.Fprintf(&b, "Connected project %s.\n", d.Account)
	}
	fmt.Fprintf(&b, "Workload identity provider: %s\n", d.WorkloadIdentityProvider)
	for _, w := range d.Warnings {
		fmt.Fprintf(&b, "\nWarning: %s\n", w)
	}
	return b.String()
}
