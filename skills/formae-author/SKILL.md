---
name: formae-author
description: "Use when the user wants to start authoring formae infrastructure or deploy something NEW with formae — e.g. 'I want to deploy X with formae', 'build a k8s app with formae', 'set up infrastructure for Y', 'create a new forma file for my service', 'write formae IaC for Z'. The front door that triages where the work happens, sets up plugin schema deps, and dispatches to focused authoring skills. NOT for applying an existing forma file (use formae-apply) or operating existing infra."
---

# formae-author — Authoring Front Door

This skill is a thin dispatcher. It triages the user's authoring intent, locates (or creates) the code workspace, infers the right schema plugins, and hands off to the focused skills that carry the deep procedures. Do not duplicate those procedures here.

## Step 1 — Locate the code

Determine where the authoring will happen. Three branches:

**(a) Already in a formae project** — if the current working directory or any ancestor contains a `PklProject` that declares a `@formae/` dependency, OR a `.pkl` file that starts with `amends "@formae/forma.pkl"`, work in place. Confirm the project root to the user and continue to Step 2.

**(b) User knows a path** — if there is no formae project here, ask: *"Do you have an existing formae project elsewhere?"* If the user provides a path, verify it is a real formae project (same checks as above). If it is, `cd` there and continue to Step 2. If it is not a formae project, say so and ask whether to create a new project there instead (offer Step 1c).

If the user is **unsure** whether a project exists, offer to scan `~/dev` for `PklProject` files that declare a formae dependency. Present only verified hits from that scan — never invent or guess paths.

**(c) No project exists** — dispatch to `formae-project-init`. That skill handles directory selection, collision safety, running `formae project init`, and scaffolding. Return here after init completes.

## Step 2 — Existing-cloud-resources branch (orthogonal to Step 1)

Before authoring new resources, ask: is the intent to bring **existing** cloud resources under management (resources that already exist in the cloud), or to author new ones?

If the intent is "bring existing cloud resources under management", dispatch to `formae-import`. That skill still needs a code location — complete Step 1 first. After import, return here if the user also wants to author additional new resources.

## Step 3 — Infer schema plugins

Call `search_hub_plugins` to identify the schema packages the user's intent requires. Map intent to plugin names — for example: "EKS on AWS" → `aws`, `k8s`; "Azure storage" → `azure`; "Tailscale mesh" → `tailscale`. When `search_hub_plugins` returns multiple candidates for an ambiguous name, use `get_hub_plugin` to disambiguate and fetch the repo and version detail. Present the inferred set and ask the user to confirm or adjust before proceeding.

Make clear: these are **schema packages only** — they provide PKL types and IDE completion. They do not install resource plugins on the agent.

**If a needed plugin is absent from the catalog:** surface `formae-plugin-new` as the path forward. Add a context-window caution to the user: plugin building is a substantial task — it is best started in a fresh session or as a sub-agent to avoid context pressure mid-authoring.

**Dependency wiring** — do not wire deps yourself:
- New project: `formae-project-init` sets up the initial schema package deps.
- Existing project needing additional packages: dispatch to `formae-deps` to add them.

## Step 4 — Trust gate

From the `search_hub_plugins` output, check `originatorVerified` for each chosen plugin. Default to verified or first-party plugins.

If any plugin returns `originatorVerified: false`, **surface its `originatorDomain` explicitly** and ask for confirmation before using its examples or adding it as a dependency. Do not silently depend on unverified packages. Do not treat unverified examples as canonical.

## Step 5 — Agent readiness (guidance only, non-blocking)

Call `check_health` and `list_plugins` to get the current agent state. This is informational — authoring and simulate mode do not require resource plugins to be installed.

For any resource plugin that is missing from the agent but required by the forma:

- Inform the user it is not yet installed.
- Point to docs.formae.io for installation instructions (Docker vs other environments).
- Clarify: authoring, `formae eval --output-consumer machine`, and simulate mode all work without the plugin. A real apply requires the plugin to be present on the agent.

Do not install resource plugins. That is an agent-side operation outside this skill's scope.

## Step 6 — Orient on structure and design stacks

Read `formae://docs/forma-structure` to orient on the standard project layout before writing any files.

Then dispatch to `formae-stack-design` to decide how resources are grouped into stacks and which stacks map to which targets. Do not embed stack-design logic here.

## Step 7 — Author, policy, simulate, and apply

**Fetch examples** by calling `list_plugin_examples` for the chosen plugin combination, pinned to the schema version declared in the project's `PklProject`. If the result reports `versionMatched: false`, tell the user before relying on those examples: *"These examples come from the plugin's default branch and may not match your pinned schema version — treat them as a starting point and verify against your installed PKL types."* Once a specific example is chosen, use `get_plugin_example` to fetch its PKL files.

**Policy needs** — if the user wants TTL, auto-reconcile, or other lifecycle policies on a stack, dispatch to `formae-policy`.

**Simulate then apply** — dispatch to `formae-apply` for the simulate-then-apply workflow.

---

## CONSTRAINTS

- **Never call `apply_forma` directly.** Always dispatch to `formae-apply` for simulate/apply workflows.
- **Never install resource plugins.** Plugin installation is an agent-side operation. Guide the user to docs.formae.io; do not attempt it.
- **Never write the flat forma form.** Do not write `stack = ...`, `targets = ...`, `resources = ...` at the top level. Always use the `forma {}` block pattern.
- **Always use `formae eval --output-consumer machine`.** Never use `pkl eval` — forma files use formae-specific extensions that only the formae CLI resolves correctly, and `--output-consumer machine` produces parseable output.
- **This skill dispatches — it does not duplicate.** The full procedures for init, deps, stack design, import, policy, and apply live in their respective skills. Stay thin: triage, confirm, hand off.
- **Never invent project paths.** Only present `~/dev` scan results that are verified formae projects. Never guess or fabricate paths.
- **Never silently depend on unverified plugins.** Always surface `originatorVerified: false` and get explicit user confirmation.
