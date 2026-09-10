---
name: formae-author
description: "Use when the user wants to start authoring formae infrastructure or deploy something NEW with formae — e.g. 'I want to deploy X with formae', 'build a k8s app with formae', 'set up infrastructure for Y', 'create a new forma file for my service', 'write formae IaC for Z'. The front door that triages where the work happens, sets up plugin schema deps, and dispatches to focused authoring skills. NOT for applying an existing forma file (use formae-apply) or operating existing infra."
---

# formae-author — Authoring Front Door

This skill is a thin dispatcher. It triages the user's authoring intent, locates (or creates) the code workspace, infers the right schema plugins, and hands off to the focused skills that carry the deep procedures. Do not duplicate those procedures here.

## Step 1 — Select the source context

Call `get_codebase_context` with the chosen `profile` and the actual harness `working_directory`. Never assume the MCP process directory is the workspace. An explicit existing project supplied by the user is an opt-in: validate it and call `register_codebase`, then select its binding. An explicit `mode: none` wins even when projects are registered.

Use the returned selection:

- `codebase`: work only in that selected registered project; carry `context: {mode: "codebase", binding_id: ...}` on apply and policy planning calls. Preserve its abstractions and unrelated edits. Profile names can change; bindings identify the resolved installation.
- `none`: hosted users with no registered codebase start here naturally. Continue authoring without asking for a project directory. Pkl is still the code interface: create a harness-owned empty disposable directory, call `prepare_authoring` with complete existing `stacks` and/or `new_stacks`, plus configured `targets` needed for new resources. Use returned full source files, PklProject and `context`. Wire any new schema dependencies from the reported installed versions and resolve the project. Keep the directory through preview, decisions and command outcome; remove after terminal outcome or abandoning a preview with no real submission. Never create a persistent hidden project or register this temporary directory.
- `selection_required`: present the registered candidates once and let the user choose one or explicitly choose none. Missing directories remain visible; never silently switch away and claim source synchronization.
- `unconfigured`: classic installations preserve the choice of an existing project or explicit none. The latter requires connected `desired-stack-extraction` and `shared-drift-resolution` capabilities.

Inspect `prepare_authoring` diagnostics before changing the extracted source. Desired declarations can retain failed intent and broken references. Explain unresolved references and repair them according to the user's request: remove the owning declaration, rewire its reference, or explicitly restore its dependency. Ask when the intended repair is unclear. Preserve original resource identity; never bind a broken reference to a same-name replacement automatically. Repair placeholders deliberately prevent Pkl evaluation until resolved. Keep unrelated declarations and drift unchanged.

A corrupt registry is an actionable error, not an empty registry. Do not scan arbitrary directories for projects. A server lacking the new capabilities requires an existing complete codebase; a newer local CLI alone does not establish capability.

If the user says they want to keep IaC locally, hand off to `formae-project-init`. This opt-in can happen at any time. Initialize their selected project, extract selected managed stacks completely (or initialize with the target on an empty installation), verify it and register it only after successful initialization. Do not ask about a maintained project as a prerequisite for hosted authoring.

## Step 2 — Existing-cloud-resources branch (orthogonal to Step 1)

Before authoring new resources, ask: is the intent to bring **existing** cloud resources under management (resources that already exist in the cloud), or to author new ones?

If the intent is "bring existing cloud resources under management", hand off to the `formae-import` skill. Carry the selected context; a disposable workspace also supports explicit import. After import, return here if the user also wants to author additional new resources.

## Step 3 — Establish what plugins are available

**Always call `list_agent_plugins` first**, passing the `profile` explicitly. It reports both the plugins the connected installation has and whether that installation is hosted formae or a self-hosted agent. The mode decides everything below, so it cannot be inferred before this call.

**Hosted.** The reported resource plugins are the catalogue. Map the user's intent onto them and confirm the set with the user before proceeding. Wire deps from the schema versions it reports.

- If the intent needs a plugin that is not in the list, say it is **not reported as installed on this installation**, and that hosted formae cannot install plugins on demand. Then say what the available set can do for the intent, and stop there.
- Do **not** call `search_hub_plugins` in this branch. The catalogue lists plugins this installation cannot install, and showing them offers something that cannot be delivered.
- Do **not** hand off to `formae-plugin-new` as a way to get a missing plugin. Authoring one does not make it installable here.
- If the listing could not be read, the tool says so. Say the available set could not be established and stop; do not fall back to the hub.

**Self-hosted.** Call `search_hub_plugins` to identify the schema packages the user's intent requires. Map intent to plugin names — for example: "EKS on AWS" → `aws`, `k8s`; "Azure storage" → `azure`; "Tailscale mesh" → `tailscale`. When `search_hub_plugins` returns multiple candidates for an ambiguous name, use `get_hub_plugin` to disambiguate and fetch the repo and version detail. Present the inferred set and ask the user to confirm or adjust before proceeding.

**If a needed plugin is absent from the catalog** (self-hosted only): surface `formae-plugin-new` as the path forward. Add a context-window caution to the user: plugin building is a substantial task — it is best started in a fresh session or as a sub-agent to avoid context pressure mid-authoring. A failed plugin listing changes nothing in this branch.

Make clear in both branches: these are **schema packages only** — they provide PKL types and IDE completion. They do not install resource plugins on the agent.

**Dependency wiring** — do not wire deps yourself:
- Opted-in maintained project: `formae-project-init` sets up the initial schema package deps. Disposable project: use the files and installed schema metadata from `prepare_authoring`, adding needed dependencies with `formae-deps`.
- Existing project needing additional packages: hand off to the `formae-deps` skill to add them.

## Step 4 — Trust gate (self-hosted only)

Skip this step on hosted formae. Its plugins are first-party and already installed, so there is no `originatorVerified` decision left to make, and asking for one implies a choice the user does not have.

From the `search_hub_plugins` output, check `originatorVerified` for each chosen plugin. Default to verified or first-party plugins.

If any plugin returns `originatorVerified: false`, **surface its `originatorDomain` explicitly** and ask for confirmation before using its examples or adding it as a dependency. Do not silently depend on unverified packages. Do not treat unverified examples as canonical.

## Step 5 — Agent readiness (guidance only, non-blocking)

Authoring and simulate mode need only plugin **schemas** (the PklProject dependencies), not the resource plugins themselves, so a missing resource plugin never blocks authoring.

Two different questions get confused here, so keep them apart. `list_agent_plugins` reports what the **agent** has, over its API, with no privileges needed — that is the one Step 3 uses. `formae plugin list` reports what **this machine's** package store holds, which is a different set and not what authoring cares about. Never shell out to the latter to answer the former. `check_health` remains a fine optional reachability check.

**Exception — `formae project init` does reach the agent**: a non-`@local` `--include <name>` resolves that plugin's version from the agent, so init can fail if the plugin isn't installed agent-side. The `formae-project-init` skill covers the `@local`/`--plugin-dir` fallback for offline init.

If a later `apply` fails because a required resource plugin is not installed on the agent:

- Inform the user which plugin is missing (the apply error will name it).
- **Self-hosted:** point to docs.formae.io for installation instructions (Docker vs other environments). Never install it yourself.
- **Hosted:** do not point at installation instructions. The plugin cannot be installed on that installation, so say that plainly and go back to what the reported set can do.
- Clarify: authoring, `formae eval --output-consumer machine`, and simulate mode all work without the plugin. A real apply requires the plugin to be present on the agent.

Do not install resource plugins. That is an agent-side operation outside this skill's scope.

## Step 6 — Orient on structure and design stacks

Read `formae://docs/forma-structure` to orient on the standard project layout before writing any files.

Then hand off to the `formae-stack-design` skill to decide how resources are grouped into stacks and which stacks map to which targets. Do not embed stack-design logic here.

## Step 7 — Author, policy, simulate, and apply

**Fetch examples** by calling `list_plugin_examples` for the chosen plugin combination, passing the schema version pinned in the project's `PklProject`. To obtain that version, read the `uri` field for the relevant plugin in the `dependencies` block of `PklProject` and extract the `@<version>` suffix (e.g., `k8s@0.3.2` → `"0.3.2"`). If the `uri` has no explicit version tag, fall back to reading `PklProject.deps.json` for the resolved version. Pass that string as the `version` argument to `list_plugin_examples` and `get_plugin_example` — do not omit it and let the tool default to `latestStable`, which may not match what the project pinned. If the result still reports `versionMatched: false`, tell the user before relying on those examples: *"These examples come from the plugin's default branch and may not match your pinned schema version — treat them as a starting point and verify against your installed PKL types."* Once a specific example is chosen, use `get_plugin_example` to fetch its PKL files.

**Policy needs** — if the user wants TTL, auto-reconcile, or other lifecycle policies on a stack, hand off to the `formae-policy` skill.

**Generated or rotating credentials** — if any credential in the design should be drawn by formae rather than written into the forma, or should turn over on a schedule, hand off to the `formae-secrets` skill. Reach for it whenever authoring is about to produce a hardcoded secret, a `random.password` seed pinned with `setOnce`, or a credential the user says must be rotated. It is a distinct concern from `formae-policy`: a policy governs a stack, whereas a generator is referenced by the properties that take its value.

**Simulate then apply** — hand off to the `formae-apply` skill for the simulate-then-apply workflow.

---

## Portability

Auto-activation from this skill's `description` field is a Claude Code behavior; on Codex, OpenCode, or other harnesses the user may need to invoke `formae-author` explicitly. "Hand off to the `formae-X` skill" means follow that skill's documented procedure — on harnesses without a programmatic skill-invocation primitive, read and execute the target skill's steps directly. MCP tool names (`apply_forma`, `list_stacks`, `create_inline_policy`, etc.) are protocol-level identifiers and are the same across all harnesses.

## CONSTRAINTS

- **Never call `apply_forma` directly.** Always follow the `formae-apply` skill for simulate/apply workflows.
- **Never install resource plugins.** Plugin installation is an agent-side operation. Guide the user to docs.formae.io; do not attempt it.
- **Never write the flat forma form.** Do not write `stack = ...`, `targets = ...`, `resources = ...` at the top level. Always use the `forma {}` block pattern.
- **Always use `formae eval --output-consumer machine`.** Never use `pkl eval` — forma files use formae-specific extensions that only the formae CLI resolves correctly, and `--output-consumer machine` produces parseable output.
- **This skill dispatches — it does not duplicate.** The full procedures for init, deps, stack design, import, policy, and apply live in their respective skills. Stay thin: triage, confirm, hand off.
- **Use explicit context.** Discover only registered projects using the supplied harness directory; never scan arbitrary disk or invent paths.
- **Never silently depend on unverified plugins** (self-hosted). Always surface `originatorVerified: false` and get explicit user confirmation. On hosted formae the question does not arise: the set is first-party and already installed.
- **Never offer a hosted user a plugin their installation does not have.** Not from the hub, and not by authoring one. The reported set is the catalogue, and anything outside it cannot be applied.
