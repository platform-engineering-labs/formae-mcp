---
name: formae-author
description: "Use when the user wants to start authoring formae infrastructure or deploy something NEW with formae — e.g. 'I want to deploy X with formae', 'build a k8s app with formae', 'set up infrastructure for Y', 'create a new forma file for my service', 'write formae IaC for Z'. The front door that triages where the work happens, sets up plugin schema deps, and dispatches to focused authoring skills. NOT for applying an existing forma file (use formae-apply) or operating existing infra."
---

# formae-author — Authoring Front Door

This skill is a thin dispatcher. It triages the user's authoring intent, locates (or creates) the code workspace, infers the right schema plugins, and hands off to the focused skills that carry the deep procedures. Do not duplicate those procedures here.

## Step 1 — Locate the code

Determine where the authoring will happen. Three branches:

**(a) Already in a formae project** — if the current working directory or any ancestor contains a `PklProject` that declares a `@formae/` dependency, OR a `.pkl` file that starts with `extends "@formae/forma.pkl"` (or the legacy `amends "@formae/forma.pkl"`), work in place. Confirm the project root to the user and continue to Step 2.

**(b) User knows a path** — if there is no formae project here, ask: *"Do you have an existing formae project elsewhere?"* If the user provides a path, verify it is a real formae project (same checks as above). If it is, `cd` there and continue to Step 2. If it is not a formae project, say so and ask whether to create a new project there instead (offer Step 1c).

If the user is **unsure** whether a project exists, offer to scan `~/dev` for `PklProject` files that declare a formae dependency. Present only verified hits from that scan — never invent or guess paths.

**(c) No project exists** — hand off to the `formae-project-init` skill. That skill handles directory selection, collision safety, running `formae project init`, and scaffolding. Return here after init completes.

## Step 2 — Existing-cloud-resources branch (orthogonal to Step 1)

Before authoring new resources, ask: is the intent to bring **existing** cloud resources under management (resources that already exist in the cloud), or to author new ones?

If the intent is "bring existing cloud resources under management", hand off to the `formae-import` skill. That skill still needs a code location — complete Step 1 first. After import, return here if the user also wants to author additional new resources.

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
- New project: `formae-project-init` sets up the initial schema package deps.
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
- **Never invent project paths.** Only present `~/dev` scan results that are verified formae projects. Never guess or fabricate paths.
- **Never silently depend on unverified plugins** (self-hosted). Always surface `originatorVerified: false` and get explicit user confirmation. On hosted formae the question does not arise: the set is first-party and already installed.
- **Never offer a hosted user a plugin their installation does not have.** Not from the hub, and not by authoring one. The reported set is the catalogue, and anything outside it cannot be applied.
