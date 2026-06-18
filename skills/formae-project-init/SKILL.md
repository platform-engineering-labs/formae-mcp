---
name: formae-project-init
description: "Use when the user wants to start a brand-new formae project from scratch (no existing IaC codebase) — scaffolds a project with `formae project init`, infers the plugin schema dependencies from the user's intent, and sets up the file structure."
---

# Initialize a New Formae Project

Scaffold a brand-new formae project from zero: infer schema plugin dependencies, preflight the target directory, run `formae project init`, and set up the standard file structure.

## Step 1 — Confirm there is no existing formae project

Before doing anything, check whether a formae project already exists in the current working directory or any stated target path:

- Look for a `PklProject` file that declares a `@formae/` dependency.
- Look for any `.pkl` file that starts with `amends "@formae/forma.pkl"`.

If either is found, **stop**. Tell the user a formae project already exists here and offer to continue editing it in place (formae-import, formae-patch, or formae-apply skills may apply). Do not proceed with init.

## Step 2 — Infer schema plugin dependencies from intent

Use `search_hub_plugins` to identify the schema packages needed. Map the user's intent to plugin names — for example:

- "EKS on AWS" → search for `aws`, `k8s`
- "Azure storage" → search for `azure`
- "Tailscale network" → search for `tailscale`

Present the inferred set to the user and ask them to confirm or adjust before proceeding.

Make clear: these are **schema packages only** — they provide PKL types and IDE completion. They do not install resource plugins on the agent, and they do not add a formae root.

**Trust gate.** For each inferred plugin, check the `originatorVerified` field from `search_hub_plugins`. If any plugin returns `originatorVerified: false`, surface the originator domain explicitly to the user and ask for confirmation before including it as a dependency. Do not silently depend on unverified packages.

## Step 3 — Preflight the target directory (collision safety)

Default to creating a **new named project directory** (e.g., `./<project-name>/`) rather than initializing in the current working directory. This avoids accidental clobbering of existing files.

Before writing anything, check for the paths that `formae project init` and scaffolding would create:

- `PklProject`
- `main.pkl`
- `<project-name>.pkl` (the sibling resources file)
- `vars.pkl`
- `modules/`

If **any** of these already exist at the target path, **stop and ask**. Never overwrite user files.

Special case: if `PklProject` exists but `main.pkl` does not, a prior init likely ran but was interrupted or incomplete. Reconcile this with the user — ask whether to resume from this state or start fresh elsewhere — rather than blindly re-scaffolding.

## Step 4 — Run `formae project init`

Show the exact command to the user before running it. Include the chosen plugin dependencies and the target directory. Example:

```
formae project init --name <project-name> --deps @formae/aws,@formae/k8s ./<project-name>/
```

Wait for confirmation, then run the command.

## Step 5 — Scaffold the project structure

After init completes, lay out the standard structure per `formae://docs/forma-structure`. Read that doc resource before writing files.

Key conventions:

- **`main.pkl`** — the root forma file; amends `@formae/forma.pkl` and contains the `forma {}` block.
- **Sibling resources file** (e.g., `<project-name>.pkl`) — holds resource declarations; imported and spread into `main.pkl`.
- **`vars.pkl`** — shared scalars and the `Target` definition. Reference from both `main.pkl` and resource files.
- **`modules/`** — only create this directory if there are 2 or more consumers of the same abstraction. Do not create it preemptively.

Consult `formae://docs/stack-design` before deciding on stacks. Ask the user how they want resources grouped if it's not obvious from the intent.

## Step 6 — Agent readiness note

Resource plugins must be installed on the **agent machine**, not by the assistant. This is out of scope for this skill.

Inform the user:

- Authoring and `formae eval --output-consumer machine` work without resource plugins installed.
- Simulate mode (`apply_forma` with `simulate: true`) also works once the agent is running.
- A real `apply` requires the relevant resource plugin to be present on the agent. See `formae-plugin-new` skill or docs.formae.io for how to install plugins.

## Step 7 — Hand back

Once the scaffold is in place:

1. Suggest pulling a **version-matched** example for the chosen plugins via `list_plugin_examples`. This gives idiomatic starting code aligned with the installed schema version.
2. Offer to design the stack layout with the stack-design skill — how resources are grouped, which stacks map to which targets.

---

## CONSTRAINTS

- **Never install resource plugins.** Plugin installation is an agent-side operation outside this skill's scope.
- **Never write the flat forma form.** Do not write `stack = ...`, `targets = ...`, `resources = ...` at the top level. Always use the `forma {}` block pattern.
- **Always use `formae eval --output-consumer machine`.** Never use `pkl eval` — forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` produces parseable output.
- **Never overwrite existing user files.** Always preflight; always stop and ask if a collision is detected.
- **Never silently depend on unverified plugins.** Surface `originatorVerified: false` to the user and get explicit confirmation.
