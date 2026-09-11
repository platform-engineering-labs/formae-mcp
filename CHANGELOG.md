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

- Local installation-scoped drift preferences: ask each time (default), or automatically keep nonconflicting external changes. Intentional patches and uncertain history still require a decision. The agent must supply proof of external-only history; older agents continue to prompt.
- MCP workflow analytics for effective codebase mode and drift preference, using the existing PostHog destination and honoring the CLI usage-reporting opt-out. Events contain categorical settings and an installation-derived identifier, not source paths, resource properties or command messages.

### Fixed

- Drift rejections now include keep/revert guidance and the current local preference in the tool response, in both codebase modes. Adding a resource property does not imply acceptance of unrelated drift. Combined changes use reviewed reconciliation; agents without that protocol produce an explicit limitation rather than a force workaround. Successful previews remind the harness to offer an editable or empty command message.

- Inventory Pkl exports now carry explicit actual-state/partial-source metadata and a warning alongside the unchanged raw Pkl. Existing-stack authoring points to complete desired extraction, preserving unabsorbed drift as an explicit reconcile decision.

- Source selection precedes IaC file access and is reused for the same installation/workspace. No-codebase authoring uses a fresh operation project under `~/.formae-ai/scratch/` and desired extraction instead of repeatedly searching for local projects or treating stale files as desired intent.

- Cloud inventory questions default to formae queries even when the user names only a provider. Results include managed and discovered resources unless narrowed, and empty inventory is distinguished from an empty cloud account.

- Ordinary resource edits, including a single label, consistently use complete-stack soft reconcile. Patch guidance now requires an explicit patch request or incident/hotfix intent, preserving drift decisions and selected source context.

- Without a maintained codebase, setup and infrastructure workflows describe resources, changes and outcomes while keeping disposable source preparation internal. Initial target creation no longer requires a persistent project or placeholder stack. Maintained-codebase workflows continue to report source changes and conflicts.
- Drift prompts distinguish changes detected outside formae from temporary patches made through formae, and separate keep/revert decisions from newly requested changes.
- The MCP reports its selected formae executable to the assistant, including managed installations outside the shell PATH. Schema lookup guidance uses exact installed versions instead of broad searches through local caches.

### Added

- Work with infrastructure without maintaining a local IaC project. Hosted authoring uses disposable complete desired Pkl source and schema dependencies; keeping a project is an explicit opt-in. Local project registration and per-call selection keep concurrent projects and installations independent.
- Resolve drift centrally with explicit absorb/revert choices, a combined final preview, recorded review identity and idempotent submission. Maintained-source catch-up uses the command's recorded desired contributions and reports local conflicts separately from the infrastructure outcome. These workflows require support from the connected agent.
- Policy planners accept a per-call profile and selected source context, including disposable Pkl workspaces. Command intent messages can be edited or deliberately cleared during ordinary final confirmation.

### Fixed

- Real apply commands with a synchronous no-change response are accepted instead of reported as failures.

## [0.9.2]

### Added

- Hosted formae users can ask their assistant to prepare and submit bug reports to support@formae.ai for likely formae, plugin, or MCP defects in any workflow. Reports include the identifiers support needs to find hosted logs and relevant sanitized local CLI diagnostics, including failures before a command reaches the agent. The assistant shows the report before sending unless the user has already authorized reporting for the session. Self-hosted users are directed to GitHub or Discord.

### Fixed

- Assistants now recognize a `Rejected` resource update as protection against a newly detected out-of-band cloud change, even when the command reports `Failed`. Guidance directs them to review the refreshed state, simulate again, and resolve whether to absorb or overwrite the change before retrying. A successful simulation does not replace that decision. Dependents skipped because of the rejection are distinguished from independent failures, avoiding unnecessary troubleshooting.


## [0.9.1]

Requires formae 0.89.0 or newer.

### Fixed

- The launcher no longer writes installer output to stdout. On a first launch, where the MCP binary and a matched `formae` still have to be downloaded, both the package manager and the hub bootstrap script printed progress and a PATH notice onto the same stream the MCP protocol uses, putting nine non-JSON lines in front of the first response. Clients that skip leading noise connected anyway, which is why it survived: it reproduces only against an empty `~/.formae-ai/opt`, never on a machine that has launched the server before.

- The Cursor install guide cloned the repository to `~/.cursor/formae-mcp` while registering the launcher from `~/.cursor/formae`, so following both steps left the configured path pointing at nothing. It now uses `~/.cursor/formae` throughout, and links the skills as `~/.agents/skills/formae`, which is also what the Codex guide does. That makes good on something the guide already claimed, that one symlink serves both.

- The `formae-connect` skill said `discoverable` defaults to `false` on a target and warned that omitting it yields a target that discovers nothing. It defaults to `true`. The `false` default belongs to the agent's own configuration schema, which declares a separate `Target` class that does not govern a forma.

## [0.9.0]

Requires formae 0.89.0 or newer.

### Added

- `list_generators`, which reports the credentials formae draws and rotates: each generator's cadence, when it last rotated, and the resources that take their value from it. The drawn value is never among them, so asking what rotates and when never risks printing a secret. An agent older than the feature reports none rather than failing, because none is the truthful answer.

- A `formae-secrets` skill, covering the three layers of a managed credential: referencing a secret's value from another resource (`secret.res.secretValue`, `.at("key")`, `.json("path")`), having formae draw the value instead of writing it into the forma, and rotating it on a cadence. It covers declaring a generator, binding a property to one of its outputs, and reading rotation state, plus the constraints that actually bite: every destination of a drawing generator has to be in the same apply, the cadence floor is fifteen minutes and is enforced at evaluation, and omitting a cadence draws once and never rotates, which is the direct replacement for a `random.password` seed pinned with `setOnce`. It also states the consumer contract that rotation cannot remove: a rotation moves the secret and, on formae 0.89.0 or newer, everything consuming it by reference in the same command, but no ordering avoids the moment where the secret and the authenticating system briefly disagree, so whatever reads the credential must re-read it at least once per rotation period and tolerate transient authentication failures around a rotation. And it explains where the floor comes from: AWS Secrets Manager keeps every version from the last 24 hours against a non-adjustable quota of 100 per secret, so a faster cadence would exhaust the quota and every rotation after that would fail. `formae-author` hands off to it whenever authoring is about to produce a hardcoded secret or the user asks for something to be rotated.
- Connect a cloud account to a hosted installation from the conversation, for AWS, GCP and Azure. `/formae:connect` asks which cloud and then follows that cloud's own path, so the tools behind it differ by provider.

  For AWS there are two routes. With local AWS credentials, `list_aws_profiles` shows your profiles next to the account each one resolves to (or the reason one could not be resolved, such as an expired SSO session), so picking one is an informed choice about where trust gets provisioned, and `provision_cloud_role` then creates the role and registers it in a single step, with no console and no waiting. Without them, `connect_cloud_account` computes the CloudFormation console link that creates the role formae assumes; you apply the stack yourself, in your own browser, and `register_cloud_role` records the role it produced.

  For GCP, `connect_gcp_project` registers a project, with an opt-in Google sign-in for the case where the agent runs somewhere other than the machine you are signing in from. For Azure, `connect_azure_subscription` registers a subscription, either with local `az` credentials or without them, and accepts a tenant hint where your account spans more than one.

  `list_cloud_connections` reports what an installation already has, so setup can check whether there is an account to manage before calling the job done and offer to connect one if not. Re-running a connection that already exists is harmless. Throughout, a result is reported as registered, never as verified or working, and if the listing itself could not be read it says so rather than reading that as no account.

- Hosted formae support, end to end. A profile whose `cli.connection` is a `Hosted` connection routes to its installation behind the shared endpoint and carries a credential, so every tool works against a hosted installation the way it does against a self-hosted agent. Results say which installation answered, in a separate block alongside the payload, and a change that fails after it was already sent says so rather than implying nothing happened, so you know whether to go and check. Requires formae 0.89.0 or newer.

  Signing in happens in the conversation. `login` returns the URL (or device code) to give the user and `complete_login` finishes once they have; signing in again while a session is open is harmless.

  A machine with nothing configured is now asked, not assumed. Reaching for any tool where there is no formae configuration asks whether you are on the hosted platform or running your own agent, instead of silently creating a local-agent profile on your behalf. The question comes before anything is resolved, because resolving is what would answer it for you.

  `/formae:setup` works from that starting point. It finds out what the machine holds, signs in, confirms what was written, and only then checks the agent, so a machine with no agent yet no longer reports a setup failure that is not about setup. It signs in before it mentions the console and comes back afterwards: if you were invited to an organization your first sign-in already covers an agent and you are never sent to the console at all, and if you have no organization yet it sends you there to create one and then picks the new agent up itself rather than ending on a link. The link is marked as coming from your assistant, so the console stops closing your signup by telling you to run `/formae:setup` when you are already inside it.

- `list_agent_plugins`, which reports the resource plugins the connected installation actually has, and whether that installation is hosted formae or a self-hosted agent. On hosted the set is closed, because plugins ride the agent image and cannot be installed on demand, so authoring now works from what is really there instead of from the hub catalogue. For each plugin it names the schema package version to pin, which is not always the installed version: a `-dev` build publishes its schema over the canonical release coordinate, so pinning what is installed would name a package that has never existed.

- Codex and Cursor are supported alongside Claude Code and OpenCode. The plugin ships a Codex manifest and MCP configuration, so installing it there is two commands rather than a hand-written config block, and there is an install guide for Cursor. Both install through the same launcher as every other harness, so they get the same prebuilt binaries and the same upgrade path.

- `get_command_status` can wait for a command to finish instead of returning whatever its state is at that instant, so asking what happened after an apply no longer means polling.

- The MCP now warns when the connected formae agent is newer than your local `formae`, so you can tell when authoring may not reflect the agent's latest capabilities. The notice points at `/formae:upgrade`, which fetches the newer `formae` after you confirm (never silently in classic mode).

### Changed

- Plugin renamed from `formae-mcp` to `formae`; added `/formae:setup` and `/formae:upgrade`.

- The plugin now downloads its prebuilt `formae-mcp` and a matched `formae` into `~/.formae-ai/opt` on first run (no build-from-source; set `FORMAE_MCP_DEV=1` for local dev builds).

- Commands issued through the MCP (apply, destroy, cancel, status, list) now identify with your machine's formae client ID (`~/.pel/formae/cli_client_id`) instead of a fixed `formae-mcp` identity, so the agent attributes them to the same client as your own `formae` runs and `client:` queries can tell one machine from another. The file is created by formae itself, which every tool call runs to resolve its connection before the ID is read, so the CLI and the MCP always share one identity rather than the MCP writing a second one that could disagree. There is no placeholder identity: when the ID is missing, unreadable or malformed, the tool call fails and says how to recreate it, because a constant would report every machine under one name and could not be told apart from an older MCP that sent no real ID at all.

- Configuration and credentials now come from a single `formae connection resolve` per tool call, replacing `formae profile show`, so the MCP and your own `formae` runs always agree on where a profile points and a request can never combine one profile revision's endpoint with another's credential. Requires formae 0.89.0 or newer.

- An expired hosted credential is refreshed and the call retried once, but only for reads. A change that fails with an expired credential refreshes it for next time and reports the failure rather than being sent twice.

- On a hosted profile, "not found" from the shared endpoint is now explained rather than passed on. It answers that way for an installation it can no longer route to, one that was suspended or destroyed, or whose subscription lapsed, which can happen part-way through a long session. Every tool now says so, instead of reporting an empty result, an unhealthy agent, or a command that was never missing.

- When several profiles exist and none is named, a hosted call now lists the candidates and asks for the `profile` argument instead of guessing.

- Every agent request is built by one internal executor, so cancellation and timeouts apply uniformly across every tool.

- The plugin no longer installs a second `formae` alongside one you already have. On launch it looks for yours (`PATH`, then `/opt/pel/bin`, `/usr/local/bin`, `~/.local/bin`, `~/bin`) and uses it; it downloads one into `~/.formae-ai/opt` only when the machine has none. Previously it downloaded a copy on every launch and then ran whichever `formae` came first on `PATH`, so the downloaded one was usually dead weight, and `/formae:upgrade` could upgrade a copy the plugin was not running. Installs are compared by their resolved location, so a symlink pointing into the managed tree, or a home directory that is itself a symlink, is not mistaken for a second install that the plugin then declines to upgrade.

- The version-skew notice now says which upgrade applies: `/formae:upgrade` for the copy the plugin installed, or the path of your own install, which the plugin will not change.

- When your `formae` is older than the MCP requires, the error names which binary is too old and whether the plugin installed it, so `/formae:upgrade` can act instead of reporting that there is nothing to do.

- Set `FORMAE_BIN` to run a specific `formae` build; it is used verbatim and treated as your own install, so nothing upgrades it behind your back.

- Tool responses are scrubbed of credentials and bounded in size. A value that came from a profile or a sign-in is removed from anything the MCP returns, including the output of `formae extract`, and a response too large to send is truncated after scrubbing rather than before, so bounding can never expose something scrubbing would have removed. A partial write counts as sent, so a truncated response is never silently retried.

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
