package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/platform-engineering-labs/formae-mcp/internal/clientid"
	"github.com/platform-engineering-labs/formae-mcp/internal/codebase"
	"github.com/platform-engineering-labs/formae-mcp/internal/config"
	"github.com/platform-engineering-labs/formae-mcp/internal/execctx"
	"github.com/platform-engineering-labs/formae-mcp/internal/featuregate"
	"github.com/platform-engineering-labs/formae-mcp/internal/formaebin"
	"github.com/platform-engineering-labs/formae-mcp/internal/profile"
	"github.com/platform-engineering-labs/formae-mcp/internal/secret"
	"github.com/platform-engineering-labs/formae-mcp/internal/tools"
	"github.com/platform-engineering-labs/formae-mcp/internal/version"
)

const serverName = "formae-mcp"

// implementation describes this server in the MCP handshake. The version is
// read from internal/version so the handshake and the CLI --version flag share
// a single, build-injectable source of truth.
func implementation() *mcp.Implementation {
	return &mcp.Implementation{
		Name:    serverName,
		Version: version.String(),
	}
}

// contextResolver is the seam server tests substitute. The concrete resolver
// lives in execctx and its injection points are unexported there.
type contextResolver interface {
	// Resolve produces the frozen context for a call. forceRefresh is for the
	// 401 path, which re-resolves with a fresh credential and then checks that
	// the target did not move.
	Resolve(ctx context.Context, profileName string, forceRefresh bool) (execctx.Context, error)
	Bin() string
	// Managed reports whether the resolved formae is the copy we provisioned,
	// which decides whether an upgrade needs sudo.
	Managed() bool
}

// Server wraps the MCP server and the formae API client.
type Server struct {
	mcpServer      *mcp.Server
	hub            *HubClient
	forcedEndpoint string             // when set, empty-profile calls use this (tests / explicit)
	ctxResolver    contextResolver    // resolves per-call execution context
	clientID       *clientid.Resolver // resolves the Client-ID header value
	// newClient builds the agent client for a resolved context. It is a field
	// for the same reason ctxResolver is: the hosted arm validates its endpoint
	// against a compile-time origin, so without a seam no test can exercise a
	// hosted handler at all. Production never replaces it.
	newClient func(execctx.Context) (*FormaeClient, error)
	// gate reports whether this machine has usable formae configuration. It is
	// a field for the same reason newClient is: it runs before resolution, so a
	// test that stubs the resolver cannot reach that stub while the real gate
	// stands in front of it, and the real gate only passes on a machine that
	// already has formae configured. Production never replaces it.
	gate func() error
	// codebaseRegistry resolves local storage, never an active project.
	codebaseRegistry  func() (codebase.Registry, error)
	workflowTelemetry *workflowTelemetry

	// loginState holds the sign-in a user is part-way through. It is the one
	// piece of state this server keeps between calls, and it exists because the
	// auth plugin's pending login is in-process memory that no second process
	// could resume.
	loginState
	reportState     reportState
	reportTransport http.RoundTripper
}

// New creates a new formae MCP server connected to the given agent endpoint.
func New(endpoint string) *Server {
	resolver := execctx.NewResolver(formaebin.NewBinResolver())
	mcpServer := mcp.NewServer(
		implementation(),
		&mcp.ServerOptions{
			Instructions: instructionsForBinary(resolver.Bin()),
		},
	)

	s := &Server{
		mcpServer:         mcpServer,
		hub:               NewHubClient(),
		forcedEndpoint:    endpoint,
		ctxResolver:       resolver,
		clientID:          clientid.NewResolver(),
		gate:              gateStore,
		codebaseRegistry:  codebase.Default,
		workflowTelemetry: newWorkflowTelemetry(),
	}
	s.newClient = s.clientFrom

	s.mcpServer.AddReceivingMiddleware(s.captureReportEvent)
	s.registerTools()
	s.registerResources()
	s.registerPrompts()

	return s
}

// clientFor builds a FormaeClient for the given profile (empty = active/default).
func (s *Server) clientFor(ctx context.Context, profileName string) (*FormaeClient, error) {
	ec, err := s.resolveCtx(ctx, profileName)
	if err != nil {
		return nil, err
	}
	return s.newClient(ec)
}

// clientFrom builds the client for an already-resolved context.
//
// This is the one place that pairs a context with its refresher, so a 401 on
// any call re-resolves through the same seam the first resolution used rather
// than reaching past it. Handlers that resolve their own context call this
// instead of constructing a client directly.
func (s *Server) clientFrom(ec execctx.Context) (*FormaeClient, error) {
	return newClientFromCtx(ec, s.refresherFor(ec))
}

// refresherFor re-resolves the same profile with the credential refreshed.
//
// It closes over the effective profile name from the original snapshot, not
// over whatever the active pointer says at refresh time: re-resolving "whatever
// is active now" is exactly the skew that resolving configuration and
// credentials together exists to prevent.
func (s *Server) refresherFor(ec execctx.Context) refresher {
	return func(ctx context.Context) (execctx.Context, error) {
		return s.ctxResolver.Resolve(ctx, ec.ProfileName, true)
	}
}

// resolveCtx returns the immutable execution context for an optional profile.
// Resolve once at the top of a handler; thread the result through eval and
// client construction so both steps agree on the binary and the connection.
//
// The profile name is validated here — that check is pure and cheap. Version
// gating is not: config.Resolve gates the CLI at 0.89.0, which already implies
// every earlier floor, and gating twice would make one call decide the binary
// twice.
//
// When a forcedEndpoint is configured and no profile is requested, it is used
// directly (this path is exercised by tests that inject a mock agent URL).
func (s *Server) resolveCtx(ctx context.Context, profileName string) (execctx.Context, error) {
	if profileName != "" {
		if err := profile.ValidateName(profileName); err != nil {
			return execctx.Context{}, err
		}
	}
	if profileName == "" && s.forcedEndpoint != "" {
		return execctx.Context{
			Conn:      config.Classic{URL: s.forcedEndpoint},
			FormaeBin: s.ctxResolver.Bin(),
		}, nil
	}

	// Asked before anything resolves, and after the forced endpoint, which is the
	// seam a test injects a mock agent URL through and must keep working with no
	// profile store at all. The order matters in both directions and neither is
	// incidental: gating first would break every test that uses that seam, and
	// resolving first would create the profile whose absence is the question.
	if err := s.gate(); err != nil {
		return execctx.Context{}, err
	}

	ec, err := s.ctxResolver.Resolve(ctx, profileName, false)
	if err != nil {
		return execctx.Context{}, explainLapsedSession(s.explainIfTooOld(err))
	}
	captureResolvedReportContext(ctx, ec)
	return ec, nil
}

// explainIfTooOld turns a version-floor refusal into something the reader can
// act on: which binary is too old, and whose it is. Resolution fails before any
// skew notice can be produced, so without this the upgrade path has no signal
// at all and reports that there is nothing to do.
func (s *Server) explainIfTooOld(err error) error {
	if !errors.Is(err, featuregate.ErrTooOld) {
		return err
	}
	if s.ctxResolver.Managed() {
		return fmt.Errorf("%w. formae at %s is the copy this plugin installed, so run /formae:upgrade to update it",
			err, s.ctxResolver.Bin())
	}
	return fmt.Errorf("%w. formae at %s is your own install, so this plugin will not change it; run /formae:upgrade for the command",
		err, s.ctxResolver.Bin())
}

// formaeBin returns the resolved formae binary path. Use this only in handlers
// that plan local file edits and never reach the agent; anything that resolves
// an execution context takes the binary from there.
func (s *Server) formaeBin() string {
	return s.ctxResolver.Bin()
}

// Run starts the MCP server with the given transport.
// Run serves the MCP protocol until ctx is done.
//
// The context is captured because a sign-in outlives the tool call that starts
// it: the login child is tied to this one, so it ends when the server does rather
// than when `login` returns.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	s.loginMu.Lock()
	s.runCtx = ctx
	s.loginMu.Unlock()
	defer s.closePendingLogin()

	return s.mcpServer.Run(ctx, transport)
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "set_drift_preference", Description: tools.DriftPreferenceDescription, Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)}}, s.handleSetDriftPreference)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "prepare_bug_report", Description: tools.PrepareBugReportDescription, Annotations: readOnly}, s.handlePrepareBugReport)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "submit_bug_report", Description: tools.SubmitBugReportDescription, Annotations: &mcp.ToolAnnotations{IdempotentHint: true}}, s.handleSubmitBugReport)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "get_codebase_context", Description: tools.CodebaseContextDescription, Annotations: readOnly}, s.handleCodebaseContext)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "list_codebases", Description: tools.ListCodebasesDescription, Annotations: readOnly}, s.handleListCodebases)
	localRegistration := &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true}
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "register_codebase", Description: tools.RegisterCodebaseDescription, Annotations: localRegistration}, s.handleRegisterCodebase)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "unregister_codebase", Description: tools.UnregisterCodebaseDescription, Annotations: localRegistration}, s.handleUnregisterCodebase)

	// Read-only tools
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_resources",
		Description: tools.ListResourcesDescription,
		Annotations: readOnly,
	}, s.handleListResources)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_stacks",
		Description: tools.ListStacksDescription,
		Annotations: readOnly,
	}, s.handleListStacks)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_targets",
		Description: tools.ListTargetsDescription,
		Annotations: readOnly,
	}, s.handleListTargets)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_command_status",
		Description: tools.GetCommandStatusDescription,
		Annotations: readOnly,
	}, s.handleGetCommandStatus)

	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "get_command_desired_delta", Description: "Get recorded desired contributions and deletion guidance from a terminal command. Partial source-edit guidance only: never use as a complete reconcile declaration. Update only the selected maintained project, preserving abstractions and unrelated edits; report source conflicts separately from the central command outcome. Requires connected shared-drift-resolution capability.", Annotations: readOnly}, s.handleCommandDesiredDelta)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_commands",
		Description: tools.ListCommandsDescription,
		Annotations: readOnly,
	}, s.handleListCommands)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_agent_stats",
		Description: tools.GetAgentStatsDescription,
		Annotations: readOnly,
	}, s.handleGetAgentStats)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "check_health",
		Description: tools.CheckHealthDescription,
		Annotations: readOnly,
	}, s.handleCheckHealth)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_policies",
		Description: tools.ListPoliciesDescription,
		Annotations: readOnly,
	}, s.handleListPolicies)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_generators",
		Description: tools.ListGeneratorsDescription,
		Annotations: readOnly,
	}, s.handleListGenerators)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_changes_since_last_reconcile",
		Description: tools.ListChangesSinceLastReconcileDescription,
		Annotations: readOnly,
	}, s.handleListChangesSinceLastReconcile)

	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "prepare_authoring", Description: "Start here to edit an existing stack without a maintained codebase: extract its COMPLETE DESIRED Pkl and dependency project, then edit and apply with the returned context. Pass stacks as exact labels, never type/resource filters. This reads recorded desired state rather than actual inventory, so unabsorbed OOB changes and temporary patches remain decisions for soft reconcile. Do not substitute extract_resources, which exports partial actual inventory. Also supports new stacks and targets. Prepare source in an explicit empty disposable directory. The harness convention is a fresh canonical ~/.formae-ai/scratch/<operation-id>/ directory; use the returned paths and context, never search for a project or reuse another operation's scratch source. Retrieves desired declarations and exact installed plugin metadata through the resolved installation, then renders offline. Returns full file paths, never truncated source. Requires connected desired-stack-extraction and shared-drift-resolution capabilities. Local files remain until the harness removes them after outcome/retry inspection; never automatically registered.", Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)}}, s.handlePrepareAuthoring)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "extract_resources",
		Description: tools.ExtractResourcesDescription,
		Annotations: readOnly,
	}, s.handleExtractResources)

	// Signing in creates profiles and changes no infrastructure, so neither hint
	// fits: ReadOnlyHint would be a lie and DestructiveHint would warn about the
	// wrong thing.
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "login", Description: tools.LoginDescription,
	}, s.handleLogin)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "complete_login", Description: tools.CompleteLoginDescription,
	}, s.handleCompleteLogin)

	// Computing the console link mutates nothing — the CloudFormation stack is
	// applied by the user, in their own browser, under their own admin session —
	// so DestructiveHint would warn about the wrong actor. Registration writes one
	// row and is idempotent.
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "connect_cloud_account", Description: tools.ConnectCloudAccountDescription,
	}, s.handleConnectCloudAccount)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "register_cloud_role", Description: tools.RegisterCloudRoleDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleRegisterCloudRole)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "list_cloud_connections", Description: tools.ListCloudConnectionsDescription,
		Annotations: readOnly,
	}, s.handleListCloudConnections)

	// The faster path alongside the two above: when the user has local AWS
	// credentials, formae can create the connect role directly with them.
	// list_aws_profiles only reads local profiles, so ReadOnlyHint fits it the
	// way it fits list_cloud_connections. provision_cloud_role creates the role
	// (and possibly the account-global OIDC provider) immediately, with no
	// console step in between, which is what earns it DestructiveHint where
	// connect_cloud_account above does not.
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "list_aws_profiles", Description: tools.ListAwsProfilesDescription,
		Annotations: readOnly,
	}, s.handleListAwsProfiles)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "provision_cloud_role", Description: tools.ProvisionCloudRoleDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleProvisionCloudRole)

	// GCP is one tool, not three. The AWS trio exists because CloudFormation's
	// quick-create URL splits the flow into emit-then-register with a console
	// step in between; GCP has no console path at all, so provisioning and
	// registering are a single call. DestructiveHint for the same reason
	// provision_cloud_role carries it: the mutation happens immediately.
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "connect_gcp_project", Description: tools.ConnectGcpProjectDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleConnectGcpProject)

	// Azure is one tool too, the same shape as GCP: one interactive path, so
	// provisioning and registering are a single call. DestructiveHint for the
	// same reason - the mutation happens immediately.
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "connect_azure_subscription", Description: tools.ConnectAzureSubscriptionDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleConnectAzureSubscription)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "register_azure_trust", Description: tools.RegisterAzureTrustDescription,
		// Not destructive: it provisions nothing and grants nothing. The access
		// already exists, created by whoever deployed the template.
	}, s.handleRegisterAzureTrust)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_azure_trust_template", Description: tools.GetAzureTrustTemplateDescription,
		// A read: it renders a link and provisions nothing.
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, s.handleGetAzureTrustTemplate)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "list_profiles", Description: tools.ListProfilesDescription, Annotations: readOnly,
	}, s.handleListProfiles)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "current_profile", Description: tools.CurrentProfileDescription, Annotations: readOnly,
	}, s.handleCurrentProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "read_profile", Description: tools.ReadProfileDescription, Annotations: readOnly,
	}, s.handleReadProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "use_profile", Description: tools.UseProfileDescription, Annotations: &mcp.ToolAnnotations{}}, s.handleUseProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "save_profile", Description: tools.SaveProfileDescription, Annotations: &mcp.ToolAnnotations{}}, s.handleSaveProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "create_profile", Description: tools.CreateProfileDescription, Annotations: &mcp.ToolAnnotations{}}, s.handleCreateProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "delete_profile", Description: tools.DeleteProfileDescription, Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)}}, s.handleDeleteProfile)
	mcp.AddTool(s.mcpServer, &mcp.Tool{Name: "diff_profiles", Description: tools.DiffProfilesDescription, Annotations: readOnly}, s.handleDiffProfiles)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "write_profile", Description: tools.WriteProfileDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleWriteProfile)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "list_agent_plugins", Description: tools.ListAgentPluginsDescription, Annotations: readOnly,
	}, s.handleListAgentPlugins)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "search_hub_plugins",
		Description: tools.SearchHubPluginsDescription,
		Annotations: readOnly,
	}, s.handleSearchHubPlugins)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_hub_plugin",
		Description: tools.GetHubPluginDescription,
		Annotations: readOnly,
	}, s.handleGetHubPlugin)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_plugin_examples",
		Description: tools.ListPluginExamplesDescription,
		Annotations: readOnly,
	}, s.handleListPluginExamples)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_plugin_example",
		Description: tools.GetPluginExampleDescription,
		Annotations: readOnly,
	}, s.handleGetPluginExample)

	// Mutation tools
	destructive := boolPtr(true)
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "apply_forma",
		Description: tools.ApplyFormaDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive},
	}, s.handleApplyForma)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "destroy_forma",
		Description: tools.DestroyFormaDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: destructive},
	}, s.handleDestroyForma)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "cancel_commands",
		Description: tools.CancelCommandsDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleCancelCommands)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_sync",
		Description: tools.ForceSyncDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleForceSync)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_discover",
		Description: tools.ForceDiscoverDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleForceDiscover)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_check_ttl",
		Description: tools.ForceCheckTTLDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: destructive},
	}, s.handleForceCheckTTL)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "force_reconcile_stack",
		Description: tools.ForceReconcileStackDescription,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, s.handleForceReconcileStack)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_inline_policy",
		Description: tools.CreateInlinePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleCreateInlinePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "create_standalone_policy",
		Description: tools.CreateStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleCreateStandalonePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "attach_standalone_policy",
		Description: tools.AttachStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleAttachStandalonePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "detach_standalone_policy",
		Description: tools.DetachStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleDetachStandalonePolicy)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "delete_standalone_policy",
		Description: tools.DeleteStandalonePolicyDescription,
		Annotations: &mcp.ToolAnnotations{},
	}, s.handleDeleteStandalonePolicy)
}

// Tool handlers — read-only

func (s *Server) handleListResources(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListResourcesInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.ListResources(ctx, input.Query)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleListStacks(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.ListStacks(ctx)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

// handleListAgentPlugins reports the installation's plugin set and what the
// connection type means for it.
//
// The two modes fail differently on purpose. Hosted treats the listing as the
// whole catalogue, so a listing it could not read is an error: continuing would
// mean inventing a set or falling back to a hub catalogue full of plugins the
// installation cannot install. Classic loses one fact and nothing else, so it
// says so and carries on.
func (s *Server) handleListAgentPlugins(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	_, hosted := ec.Conn.(config.Hosted)

	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	plugins, err := c.ListPlugins(ctx)
	if err != nil {
		if hosted {
			return attribute(reached(ec, c), errorResult(err)), nil, nil
		}
		return attribute(reached(ec, c), textResult(renderClassicListingUnavailable(err))), nil, nil
	}
	return attribute(reached(ec, c), textResult(renderAgentPlugins(hosted, plugins))), nil, nil
}

func (s *Server) handleListTargets(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListTargetsInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.ListTargets(ctx, input.Query)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleGetCommandStatus(ctx context.Context, _ *mcp.CallToolRequest, input tools.GetCommandStatusInput) (*mcp.CallToolResult, any, error) {
	if input.CommandID == "" {
		return errorResult(fmt.Errorf("command_id is required")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	fetch := func(ctx context.Context) (json.RawMessage, error) {
		clientID, err := s.clientID.Resolve()
		if err != nil {
			return nil, err
		}
		return c.GetCommandStatus(ctx, input.CommandID, clientID)
	}

	if !input.Wait {
		result, err := fetch(ctx)
		if err != nil {
			return attribute(reached(ec, c), errorResult(err)), nil, nil
		}
		return attribute(reached(ec, c), jsonResult(result)), nil, nil
	}

	// The wait belongs here rather than in the caller. A caller polling from
	// outside has to invent a delay, and the only delay available to an agent
	// is a shell sleep, which is slower than this loop and puts commands in
	// front of the user that have nothing to do with their work.
	result, _, err := waitForCommand(ctx, resolveWaitTimeout(input.TimeoutSeconds), fetch)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleListCommands(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListCommandsInput) (*mcp.CallToolResult, any, error) {
	maxResults := input.MaxResults
	if maxResults == "" {
		maxResults = "10"
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	clientID, err := s.clientID.Resolve()
	if err != nil {
		return nil, nil, err
	}
	result, err := c.ListCommands(ctx, input.Query, maxResults, clientID)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleGetAgentStats(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.GetAgentStats(ctx)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleCheckHealth(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	if err := c.CheckHealth(ctx); err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	msg := "Formae agent is healthy and reachable."
	if notice := s.buildSkewNotice(ctx, ec.FormaeBin, c); notice != "" {
		msg += "\n\n" + notice
	}
	return attribute(reached(ec, c), textResult(msg)), nil, nil
}

// buildSkewNotice fetches the agent version and the local formae version and
// returns a skew notice when they differ, or "" when skew cannot be determined.
// It never returns an error — version-skew information is advisory only.
// The caller passes formaeBin (already resolved) to avoid a redundant resolveCtx call.
func (s *Server) buildSkewNotice(ctx context.Context, formaeBin string, c *FormaeClient) string {
	statsJSON, err := c.GetAgentStats(ctx)
	if err != nil {
		return ""
	}
	var stats struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(statsJSON, &stats); err != nil || stats.Version == "" {
		return ""
	}
	localVer, err := featuregate.DetectContext(ctx, formaeBin)
	if err != nil {
		return ""
	}
	return skewNotice(stats.Version, localVer, formaeBin, s.ctxResolver.Managed())
}

func (s *Server) handleListPolicies(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.ListPolicies(ctx)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleListGenerators(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.ListGenerators(ctx)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleListChangesSinceLastReconcile(ctx context.Context, _ *mcp.CallToolRequest, input tools.ListChangesSinceLastReconcileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}

	if input.Stack != "" {
		result, err := c.ListChangesSinceLastReconcile(ctx, input.Stack)
		if err != nil {
			return attribute(reached(ec, c), errorResult(err)), nil, nil
		}
		return attribute(reached(ec, c), jsonResult(result)), nil, nil
	}

	// No stack specified: fetch all stacks, then get drift for each
	stacksJSON, err := c.ListStacks(ctx)
	if err != nil {
		return attribute(reached(ec, c), errorResult(fmt.Errorf("failed to list stacks: %w", err))), nil, nil
	}

	var stacks []struct {
		Label string `json:"Label"`
	}
	if err := json.Unmarshal(stacksJSON, &stacks); err != nil {
		return attribute(reached(ec, c), errorResult(fmt.Errorf("failed to parse stacks: %w", err))), nil, nil
	}

	type stackDrift struct {
		Stack             string          `json:"Stack"`
		ModifiedResources json.RawMessage `json:"ModifiedResources"`
	}
	var results []stackDrift

	for _, stack := range stacks {
		driftJSON, err := c.ListChangesSinceLastReconcile(ctx, stack.Label)
		if err != nil {
			return attribute(reached(ec, c), errorResult(fmt.Errorf("failed to get drift for stack %s: %w", stack.Label, err))), nil, nil
		}

		// Parse to check if there are modifications
		var drift struct {
			ModifiedResources json.RawMessage `json:"ModifiedResources"`
		}
		if err := json.Unmarshal(driftJSON, &drift); err != nil {
			return attribute(reached(ec, c), errorResult(fmt.Errorf("failed to parse drift for stack %s: %w", stack.Label, err))), nil, nil
		}

		results = append(results, stackDrift{
			Stack:             stack.Label,
			ModifiedResources: drift.ModifiedResources,
		})
	}

	aggregated, err := json.Marshal(results)
	if err != nil {
		return attribute(reached(ec, c), errorResult(fmt.Errorf("failed to marshal results: %w", err))), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(aggregated)), nil, nil
}

func (s *Server) handleExtractResources(ctx context.Context, _ *mcp.CallToolRequest, input tools.ExtractResourcesInput) (*mcp.CallToolResult, any, error) {
	if input.Query == "" {
		return errorResult(fmt.Errorf("query is required")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	tmpDir, err := os.MkdirTemp("", "formae-extract-*")
	if err != nil {
		return errorResult(fmt.Errorf("failed to create temp directory: %w", err)), nil, nil
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	outFile := tmpDir + "/extracted.pkl"
	args := []string{"extract", "--query", input.Query, "--yes"}
	// Pin the profile the context resolved, not the global active pointer: that
	// pointer is shared with the user's CLI and other sessions and can move
	// between the two invocations this call makes.
	if ec.ProfileName != "" {
		args = append(args, "--profile", ec.ProfileName)
	}
	args = append(args, outFile)
	// Extract reaches the agent through the CLI, so do never runs and cannot
	// advance reach. It is taken from the subprocess outcome instead, and that
	// is sound here for a reason rather than by exemption: extract is a read.
	// It creates, changes and destroys nothing, so "might this have acted?"
	// has one answer however it fails, and neither wording can send an operator
	// looking for work that cannot exist. Do not copy this to a mutation.
	clientLog := snapshotClientLog(ctx, ec)
	cmd := commandWithContext(ctx, ec.FormaeBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		clientLog.capture(ctx)
		return attribute(resolved(ec),
			errorResult(fmt.Errorf("formae extract failed: %w\noutput: %s",
				err, safeSubprocessOutput(output, ec.Credential)))), nil, nil
	}
	extracted := destination{ec: ec, reach: reachAnswered}

	content, err := os.ReadFile(outFile)
	if err != nil {
		return attribute(extracted, errorResult(fmt.Errorf("failed to read extracted file: %w", err))), nil, nil
	}

	notice := ""
	if c, cerr := s.newClient(ec); cerr == nil {
		notice = s.buildSkewNotice(ctx, ec.FormaeBin, c)
	}
	result := textResult(string(content))
	// Preserve raw Pkl in the first content block for existing import/export
	// consumers. Provenance is additive and must travel with the tool result:
	// inventory source does not become desired source just because it has a
	// stack declaration or happens to contain all currently visible resources.
	result.StructuredContent = map[string]any{
		"state": "actual", "partial": true, "query": input.Query,
		"recommended_tool": "prepare_authoring",
	}
	withNotice(result, tools.ActualExtractionNotice)
	return attribute(extracted, withNotice(result, notice)), nil, nil
}

func (s *Server) handleSearchHubPlugins(_ context.Context, _ *mcp.CallToolRequest, input tools.SearchHubPluginsInput) (*mcp.CallToolResult, any, error) {
	plugins, err := s.hub.SearchPlugins(input.Query)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(plugins)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

func (s *Server) handleGetHubPlugin(_ context.Context, _ *mcp.CallToolRequest, input tools.GetHubPluginInput) (*mcp.CallToolResult, any, error) {
	if input.Name == "" {
		return errorResult(fmt.Errorf("name is required")), nil, nil
	}
	d, err := s.hub.GetPlugin(input.Name)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(d)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

func (s *Server) handleListPluginExamples(_ context.Context, _ *mcp.CallToolRequest, input tools.ListPluginExamplesInput) (*mcp.CallToolResult, any, error) {
	if input.Plugin == "" {
		return errorResult(fmt.Errorf("plugin is required")), nil, nil
	}
	result, err := s.hub.ListExamples(input.Plugin, input.Version)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

func (s *Server) handleGetPluginExample(_ context.Context, _ *mcp.CallToolRequest, input tools.GetPluginExampleInput) (*mcp.CallToolResult, any, error) {
	if input.Plugin == "" {
		return errorResult(fmt.Errorf("plugin is required")), nil, nil
	}
	if input.Example == "" {
		return errorResult(fmt.Errorf("example is required")), nil, nil
	}
	result, err := s.hub.GetExample(input.Plugin, input.Example, input.Version)
	if err != nil {
		return errorResult(err), nil, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(data), nil, nil
}

// Tool handlers — mutations

func (s *Server) handleApplyForma(ctx context.Context, _ *mcp.CallToolRequest, input tools.ApplyFormaInput) (*mcp.CallToolResult, any, error) {
	if input.FilePath == "" {
		return errorResult(fmt.Errorf("file_path is required")), nil, nil
	}
	if input.Mode == "" {
		return errorResult(fmt.Errorf("mode is required (reconcile or patch)")), nil, nil
	}
	if input.Mode != "reconcile" && input.Mode != "patch" {
		return errorResult(fmt.Errorf("mode must be 'reconcile' or 'patch', got '%s'", input.Mode)), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	formaJSON, err := s.evaluateSource(ctx, ec, c, input.Context, input.FilePath)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	if err := s.validateSourceContext(ctx, ec, input.Context, input.FilePath, formaJSON); err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	clientID, err := s.clientID.Resolve()
	if err != nil {
		return nil, nil, err
	}
	result, err := c.submitCommand(ctx, "apply", input.Mode, input.Simulate, input.Force, formaJSON, clientID, input.Resolution, input.Message)
	if err != nil {
		return attribute(reached(ec, c), s.applyErrorResult(ctx, ec, err)), nil, nil
	}
	reply := withNotice(jsonResult(result), s.keepPreferenceNotice(ctx, ec, input, result))
	if input.Simulate && input.Message == nil {
		reply = withNotice(reply, "In the final apply confirmation, suggest a concise factual command message describing the requested change and any accepted or reverted drift. The user can accept, edit or omit it in that same response. Pass the agreed message on real submission; an explicitly omitted message is an empty string. Do not infer apply confirmation solely from a message edit.")
	}
	return attribute(reached(ec, c), withNotice(reply, s.buildSkewNotice(ctx, ec.FormaeBin, c))), nil, nil
}

func (s *Server) handleDestroyForma(ctx context.Context, _ *mcp.CallToolRequest, input tools.DestroyFormaInput) (*mcp.CallToolResult, any, error) {
	if input.FilePath == "" && input.Query == "" {
		return errorResult(fmt.Errorf("either file_path or query is required")), nil, nil
	}
	if input.FilePath != "" && input.Query != "" {
		return errorResult(fmt.Errorf("file_path and query are mutually exclusive")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}

	if input.Query != "" {
		if input.Context != nil {
			return attribute(resolved(ec), errorResult(fmt.Errorf("query destruction does not accept local source context; use a file to validate selected project scope"))), nil, nil
		}
		clientID, err := s.clientID.Resolve()
		if err != nil {
			return nil, nil, err
		}
		result, err := c.DestroyByQuery(ctx, input.Query, input.Simulate, clientID)
		if err != nil {
			return attribute(reached(ec, c), errorResult(err)), nil, nil
		}
		return attribute(reached(ec, c), jsonResult(result)), nil, nil
	}

	formaJSON, err := s.evaluateSource(ctx, ec, c, input.Context, input.FilePath)
	if err != nil {
		return attribute(reached(ec, c), errorResult(fmt.Errorf("failed to evaluate forma file: %w", err))), nil, nil
	}

	if err := s.validateSourceContext(ctx, ec, input.Context, input.FilePath, formaJSON); err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	clientID, err := s.clientID.Resolve()
	if err != nil {
		return nil, nil, err
	}
	result, err := c.SubmitCommand(ctx, "destroy", "", input.Simulate, false, formaJSON, clientID)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleCancelCommands(ctx context.Context, _ *mcp.CallToolRequest, input tools.CancelCommandsInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	clientID, err := s.clientID.Resolve()
	if err != nil {
		return nil, nil, err
	}
	result, err := c.CancelCommands(ctx, input.Query, clientID)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleForceSync(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	if err := c.ForceSync(ctx); err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), textResult("Resource synchronization triggered successfully.")), nil, nil
}

func (s *Server) handleForceDiscover(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	if err := c.ForceDiscover(ctx); err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), textResult("Resource discovery triggered successfully.")), nil, nil
}

func (s *Server) handleForceCheckTTL(ctx context.Context, _ *mcp.CallToolRequest, input tools.ProfileInput) (*mcp.CallToolResult, any, error) {
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	result, err := c.ForceCheckTTL(ctx)
	if err != nil {
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(result)), nil, nil
}

func (s *Server) handleForceReconcileStack(ctx context.Context, _ *mcp.CallToolRequest, input tools.ForceReconcileStackInput) (*mcp.CallToolResult, any, error) {
	if input.Stack == "" {
		return errorResult(fmt.Errorf("stack is required")), nil, nil
	}
	ec, err := s.resolveCtx(ctx, input.Profile)
	if err != nil {
		return errorResult(err), nil, nil
	}
	c, err := s.newClient(ec)
	if err != nil {
		return attribute(resolved(ec), errorResult(err)), nil, nil
	}
	body, _, err := c.ForceReconcileStack(ctx, input.Stack)
	if err != nil {
		if body != nil {
			return attribute(reached(ec, c), errorResult(fmt.Errorf("%s: %s", err.Error(), string(body)))), nil, nil
		}
		return attribute(reached(ec, c), errorResult(err)), nil, nil
	}
	return attribute(reached(ec, c), jsonResult(body)), nil, nil
}

// maxSubprocessOutput bounds the diagnostics a failed CLI invocation may put
// into a tool result. Unlike the configuration oracle, which reports an exit
// status and never the bytes, extract's output is the user's own Pkl and
// plugin diagnostics and is the whole value of the failure, so it is kept —
// bounded, and with the credential we handed the process removed.
const maxSubprocessOutput = 8 << 10

// safeSubprocessOutput bounds subprocess output and removes the credential
// this call resolved.
//
// It covers the credential we gave the process. A credential the CLI minted
// for itself is not ours to know, and masking that is the CLI's own job on its
// own output — said plainly here because "output is sanitised" would be a
// wider claim than this makes good.
func safeSubprocessOutput(output []byte, credential secret.Value) string {
	// Scrub first, then truncate. The other order leaves a credential that
	// straddles the cutoff partly intact: the search string is no longer
	// present in the truncated bytes, so nothing is replaced and all but the
	// tail of the token survives.
	scrubbed := string(output)
	if !credential.IsZero() {
		scrubbed = strings.ReplaceAll(scrubbed, credential.Reveal(), secret.Mask)
	}
	if len(scrubbed) > maxSubprocessOutput {
		scrubbed = scrubbed[:maxSubprocessOutput] + "\n… truncated"
	}
	return scrubbed
}

// Helpers

func evalFormaFile(ctx context.Context, ec execctx.Context, filePath string) ([]byte, error) {
	if strings.HasSuffix(filePath, ".json") {
		return os.ReadFile(filePath)
	}

	args := []string{"eval", filePath, "--output-schema", "json", "--output-consumer", "machine"}
	// Same reason as extract: the active pointer can move between the two
	// invocations one call makes, so name the profile the context resolved.
	if ec.ProfileName != "" {
		args = append(args, "--profile", ec.ProfileName)
	}
	clientLog := snapshotClientLog(ctx, ec)
	cmd := commandWithContext(ctx, ec.FormaeBin, args...)
	output, err := cmd.Output()
	if err != nil {
		clientLog.capture(ctx)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("formae eval failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("formae eval failed: %w", err)
	}

	return output, nil
}

// commandWithContext builds a subprocess bound to ctx, in its own process
// group. Cancelling exec.CommandContext stops only the immediate child, and
// formae spawns plugin children that hold the output pipe open, so the call
// would go on waiting for them.
func commandWithContext(ctx context.Context, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	return cmd
}

// withNotice appends notice as a separate text content block to res when notice
// is non-empty, leaving the primary content block pristine. When notice is ""
// it returns res unchanged. This keeps structured output (JSON receipts, PKL
// code) in the first block so callers can parse it without stripping a prefix.
func withNotice(res *mcp.CallToolResult, notice string) *mcp.CallToolResult {
	if notice == "" {
		return res
	}
	res.Content = append(res.Content, &mcp.TextContent{Text: notice})
	return res
}

func jsonResult(data json.RawMessage) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Error: %s", err.Error())},
		},
		IsError: true,
	}
}
