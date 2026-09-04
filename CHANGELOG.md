# Changelog

All notable changes to `formae-mcp` (the MCP server and skills that integrate
formae with AI coding assistants like Claude Code, Codex, and OpenCode) are
documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install via the
[`platform-engineering-labs/formae-marketplace`](https://github.com/platform-engineering-labs/formae-marketplace).

## [Unreleased]

### Added

- `connect_cloud_account` and `register_cloud_role`, which connect an AWS account to a hosted installation from the conversation. The first computes the CloudFormation console link that creates the role formae assumes; you apply the stack yourself, in your own browser, and the second records the role it produced. Re-running a connection that already exists is harmless. AWS only, matching the CLI.
- `list_aws_profiles` and `provision_cloud_role`, a faster way to connect an AWS account when you have local AWS credentials. The first lists your local AWS profiles alongside the account each one resolves to (or the reason one could not be resolved, such as an expired SSO session), so picking one is an informed choice about where trust gets provisioned. The second creates the role with the chosen profile's credentials and registers it, in a single step — no console, no waiting. Unlike `connect_cloud_account`, this creates a real IAM role, and possibly an account-global OIDC provider, immediately, with no console step standing between the call and the mutation.
- `list_cloud_connections`, which reports the cloud accounts a hosted installation has registered. Setup uses it to check whether there is a cloud account to manage before it calls the job done, and offers to connect one if not. An account it names is registered, not verified; and if the listing itself could not be read, it says so rather than reading that as no account, so a caller never offers to connect one on top of a working connection.
- `/formae:connect` now opens by asking which cloud (AWS, Azure, or GCP — only AWS is implemented, and the skill says so plainly rather than promising a date for the others). For AWS it lists your local profiles first, each next to the account it resolves to, plus a "none of these" option; picking a profile provisions and registers the role directly, and "none of these," or no profiles at all, falls back to the console-link flow: it hands you the link, waits while you apply it, and registers the role once you have. Either way the result is reported as registered, never verified or working, and it tells you how to add another account afterwards.
- Onboarding for a machine with nothing configured. Reaching for any tool on a machine that has no formae configuration now asks whether you are using the hosted platform or running your own agent, instead of silently creating a local-agent profile on your behalf. The question is asked before anything is resolved, because resolving is what would answer it for you.
- `login` and `complete_login`, which sign in to the hosted platform from the conversation. `login` returns the URL (or device code) to give the user; `complete_login` finishes once they have. Signing in again while a session is open is harmless.
- `/formae:setup` now works on a machine where nothing is configured. It finds out what the machine holds, signs in, confirms what was written, and only then checks the agent — so a machine with no agent yet no longer reports a setup failure that is not about setup.
- Setup signs in before it mentions the console, and comes back afterwards. If you were invited to an organization, your first sign-in already covers an agent and you are never sent to the console at all. If you have no organization yet, setup sends you there to create one and then picks the new agent up itself, rather than ending on a link. The link it hands you is marked as coming from your assistant, so the console stops closing your signup by telling you to run `/formae:setup` — you are already inside it — and points you back to your harness instead.
- Hosted formae support. A profile whose `cli.connection` is a `Hosted` connection now routes to its installation behind the shared endpoint and carries a credential, so every tool works against a hosted installation the way it does against a self-hosted agent. Requires formae 0.89.0 or newer.
- Hosted results say which installation answered, in a separate block alongside the payload. When a change fails after it was already sent, the result says so rather than implying nothing happened, so you know whether to go and check.
- The MCP now warns when the connected formae agent is newer than your local `formae`, so you can tell when authoring may not reflect the agent's latest capabilities. The notice points at `/formae:upgrade`, which fetches the newer `formae` after you confirm (never silently in classic mode).

### Changed

- Plugin renamed from `formae-mcp` to `formae`; added `/formae:setup` and `/formae:upgrade`.
- The plugin now downloads its prebuilt `formae-mcp` and a matched `formae` into `~/.formae-ai/opt` on first run (no build-from-source; set `FORMAE_MCP_DEV=1` for local dev builds).
- Commands issued through the MCP (apply, destroy, cancel, status, list) now identify with your CLI's client ID (`~/.pel/formae/cli_client_id`) instead of a fixed `formae-mcp` identity, so the agent attributes them to the same client as your own `formae` runs. When the ID file does not exist yet, the MCP runs `formae --version` once so formae creates it, and falls back to the old `formae-mcp` identity if it still cannot be read.
- Configuration and credentials now come from a single `formae connection resolve` per tool call, replacing `formae profile show`, so the MCP and your own `formae` runs always agree on where a profile points and a request can never combine one profile revision's endpoint with another's credential. Requires formae 0.89.0 or newer.
- An expired hosted credential is refreshed and the call retried once, but only for reads. A change that fails with an expired credential refreshes it for next time and reports the failure rather than being sent twice.
- On a hosted profile, "not found" from the shared endpoint is now explained rather than passed on. It answers that way for an installation it can no longer route to — one that was suspended or destroyed, or whose subscription lapsed — which can happen part-way through a long session. Every tool now says so, instead of reporting an empty result, an unhealthy agent, or a command that was never missing.
- When several profiles exist and none is named, a hosted call now lists the candidates and asks for the `profile` argument instead of guessing.
- Every agent request is built by one internal executor, so cancellation and timeouts apply uniformly across every tool.
- The plugin no longer installs a second `formae` alongside one you already have. On launch it looks for yours (`PATH`, then `/opt/pel/bin`, `/usr/local/bin`, `~/.local/bin`, `~/bin`) and uses it; it downloads one into `~/.formae-ai/opt` only when the machine has none. Previously it downloaded a copy on every launch and then ran whichever `formae` came first on `PATH`, so the downloaded one was usually dead weight — and `/formae:upgrade` could upgrade a copy the plugin was not running. Installs are compared by their resolved location, so a symlink pointing into the managed tree, or a home directory that is itself a symlink, is not mistaken for a second install that the plugin then declines to upgrade.
- The version-skew notice now says which upgrade applies: `/formae:upgrade` for the copy the plugin installed, or the path of your own install, which the plugin will not change.
- When your `formae` is older than the MCP requires, the error names which binary is too old and whether the plugin installed it, so `/formae:upgrade` can act instead of reporting that there is nothing to do.
- Set `FORMAE_BIN` to run a specific `formae` build; it is used verbatim and treated as your own install, so nothing upgrades it behind your back.

### Removed

- `FORMAE_AGENT_URL` and `FORMAE_AGENT_PORT`. Configure a profile instead.

## [0.8.0]

### Changed

- The authoring guidance now teaches `extends "@formae/forma.pkl"` with a typed
  `properties: Props` class as the canonical forma shape, replacing the older
  `amends` + `new formae.Prop {}` form (which still works and is still detected).
  Covers member-name-as-flag, `@formae.Flag` overrides, typed reads without
  `.value`, pkl constraint validation, and per-property `--<flag>` injection at
  apply time. The self-contained destroy-forma the MCP emits also uses `extends`.
- Documentation links the assistant shares now follow the new Mintlify docs-site
  URL scheme (served at the `docs.formae.io` site root) instead of the previous
  Read the Docs paths (`/en/latest/…`), so the concept, PKL, setup, and plugin-SDK
  links keep resolving after the docs-site migration.

## [0.7.0]

### Added

- Manage standalone (reusable) policies from your assistant: create a policy once
  at the top of a forma and attach, detach, or delete it across any number of
  stacks by reference, plus a read-only view of existing policies. Each tool plans
  a file edit and returns a snippet with a line anchor rather than writing files.
  The auto-reconcile policy type requires formae 0.88.0 or newer (both standalone
  and inline, where earlier binaries dropped its label and churned a phantom
  update); standalone policies otherwise require 0.82.0, and TTL is never gated by
  the auto-reconcile floor, so a 0.82–0.87 formae keeps full TTL support. Tools
  refuse cleanly with a version message rather than degrading silently.

## [0.6.0]

### Added

- Manage named formae environments from your assistant: list them, show which is
  active, switch between them, snapshot the current one under a new name, create
  new ones from a template, delete them, compare two, and view or replace a
  profile's contents. These operations previously lived only in the separate
  `fcfg` command-line tool, which formae 0.87.0 folded into `formae profile`.
- Per-command environment targeting: apply, destroy, status, inventory, the list
  and force commands, cancel, and extract now accept an optional environment to
  run a single command against without changing the active one. The assistant
  prefers this over switching the active environment, because the active
  selection is shared with the `formae` CLI and any other assistant sessions.

### Changed

- Profile management and per-command targeting require formae 0.87.0 or newer.
  On an older formae these tools return a clear "requires formae >= 0.87.0"
  message instead of failing confusingly.

## [0.5.0] - 2026-06-22

### Added

- Author infrastructure by describing it: a guided path from a request like
  "deploy a service on AWS with formae" to idiomatic forma code, covering where
  the code should live, which plugins are needed as schema dependencies, how to
  group resources into stacks and place lifecycle policies, and simulating before
  applying. Includes workflows for starting a project, adding or removing plugin
  dependencies, and stack design.
- Live plugin catalog search and real examples pulled from each plugin's
  repository, matched to the plugin version your project pins, with a warning
  when an example may not match and a guard against treating unverified
  third-party examples as authoritative.
- Reusable (standalone) TTL and auto-reconcile policies that attach to several
  stacks, alongside the existing inline policies.
- Rename a managed resource in place via an `alias`, with no destroy-and-recreate;
  a rename combined with a change to an immutable field is flagged as destructive
  and asks for confirmation.
- `--version` (and `-V`) and `--help` on the `formae-mcp` binary, with one
  consistent version reported across the command line and the MCP handshake.

### Changed

- Skills behave consistently whether you drive formae from Claude Code, Codex, or
  OpenCode.
- Corrected and expanded the built-in authoring guidance: forma-file structure,
  stacks as the unit of reconciliation, where to find examples, and common
  pitfalls to avoid.

## [0.4.0] - 2026-06-14

### Added

- A built-in index of canonical documentation links, so links the assistant
  shares are drawn from the index rather than assembled ad hoc. Covers core
  concepts, the PKL cheatsheet, the AI-assistant setup guide, and the plugin SDK
  tutorial and reference.

### Fixed

- Plugin SDK tutorial and reference links the assistant shared previously led to
  "page not found" errors; they now open the correct pages.

## [0.3.2] - 2026-05-28

### Fixed

- Updates pulled with `/plugin marketplace update` now take effect on the next
  session start. The `start-mcp.sh` wrapper previously built the Go binary only
  on first install and kept serving the stale cached binary; it now detects
  changed source and rebuilds automatically.

> One-time catch-up: the marketplace catalog previously pinned `formae-mcp` to
> 0.2.0, which blocked `/plugin marketplace update` from delivering newer
> releases. With the pin removed, users on 0.2.0 jump directly to 0.3.2 (picking
> up 0.3.0, 0.3.1, and 0.3.2 at once) on their next update.

## [0.3.1] - 2026-05-21

### Added

- On-demand reference resources (`formae://docs/pkl-primer`,
  `formae://docs/forma-anatomy`, `formae://docs/annotations`,
  `formae://docs/troubleshooting`) so the assistant understands PKL syntax, forma
  file structure, schema annotations, and common error messages out of the box.
- Canonical [docs.formae.io](https://docs.formae.io/en/latest/) citations when
  the assistant explains a concept (stacks, targets, drift, apply modes, the
  `.res` accessor).

### Changed

- More accurate first-pass plugin scaffolds: the `formae-plugin-new` skill now
  guides assistants through advanced patterns (polymorphic resources,
  cross-plugin Target references, computed Resolvable outputs, synthetic
  identifiers, external-binary integrations such as helm or atlas).
- Install and update instructions point to `/reload-plugins` (Claude Code
  v2.1.116 and newer) to apply changes without restarting the session.

## [0.3.0] - 2026-05-19

### Added

- Manage stack policies in natural language via the `create_inline_policy` tool
  and the `formae-stack-policy` skill, for example "expire lifeline in 20 minutes"
  or "reject out-of-band changes on production".
- Switch between formae config profiles by asking, via the `formae-config` skill
  driving the `fcfg` companion command.

### Fixed

- Drift-detection workflows (`/formae-fix-code-drift` and the
  `list_changes_since_last_reconcile` tool) now return results correctly;
  earlier versions called the wrong agent endpoint and silently returned empty
  results.

### Changed

- `/formae-apply` suggests clearer recovery options when a deploy fails mid-way
  (which resources to retry, which to roll back, which to inspect first).

## [0.2.0] - 2026-02-12

Initial public marketplace release. With `formae-mcp` installed, your assistant
can:

- Inspect your infrastructure from the live formae agent ("what's running in
  production?", "any failed commands today?", "show me unmanaged resources in
  us-west-2").
- Deploy and update infrastructure through a strict simulate, confirm, apply
  loop.
- Hot-fix during incidents with patch mode, without reconciling the rest of the
  stack.
- Absorb out-of-band changes into your IaC codebase (extract current state, edit
  your PKL to match, verify with a dry run).
- Discover and import resources not yet managed by formae.
- Build new resource plugins, TDD-ing through each CRUD operation against the
  plugin SDK tutorial.

Ships with 15 MCP tools and 13 skills. License: FSL-1.1-ALv2.
