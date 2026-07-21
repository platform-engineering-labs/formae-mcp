# Changelog

All notable changes to `formae-mcp` (the MCP server and skills that integrate
formae with AI coding assistants like Claude Code, Codex, and OpenCode) are
documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install via the
[`platform-engineering-labs/formae-marketplace`](https://github.com/platform-engineering-labs/formae-marketplace).

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
