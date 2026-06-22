---
name: formae-deps
description: "Use when the user wants to add or remove a plugin schema dependency in an existing formae project's PklProject — e.g. 'add the grafana plugin', 'I need cloudflare DNS', 'drop the azure dependency'."
---

# Manage Formae Schema Dependencies

Use this skill to add or remove plugin schema dependencies in an existing formae project's `PklProject` file. This is **authoring-only** — schema packages provide PKL types and IDE completion. They do not install resource plugins on the agent and do not make infrastructure changes.

## Step 1 — Locate the project's `PklProject`

Find the `PklProject` file in the current working directory or the path stated by the user. If it cannot be found, tell the user and stop — this skill requires an existing formae project. (If they need to create one from scratch, the `formae-project-init` skill applies.)

## Step 2 — Add a dependency

### 2a — Resolve the package and version

Use `get_hub_plugin` to look up the plugin by name. If the plugin name is ambiguous or unknown, use `search_hub_plugins` first to find candidates and present them to the user for confirmation.

From the resolved plugin record, extract the latest stable version.

**Trust gate.** Check the `originatorVerified` field. If it is `false`, surface the originator domain explicitly to the user and ask for confirmation before proceeding. Do not silently add an unverified package.

### 2b — Write the dependency entry

Add the following block inside the `dependencies` section of `PklProject`:

```
["<name>"] {
  uri = "package://hub.platform.engineering/plugins/<name>/schema/pkl/<name>/<name>@<version>"
}
```

For example, for grafana at version 0.1.3:

```
["grafana"] {
  uri = "package://hub.platform.engineering/plugins/grafana/schema/pkl/grafana/grafana@0.1.3"
}
```

Read the file first, then apply the edit with the Edit tool. Show the diff to the user before proceeding.

### 2c — Resolve the deps lockfile

After editing `PklProject`, the lockfile `PklProject.deps.json` must be updated. This is a **PKL** command — `formae` has no `project resolve` subcommand (`formae project` only has `init`). Show it to the user before running it, from the directory containing `PklProject`:

```
pkl project resolve
```

### 2d — Fetch examples for the newly-added plugin

When pulling examples for the plugin just added, pass the version pinned in `PklProject` (the `@<version>` suffix from the dependency `uri`) as the `version` argument to `list_plugin_examples` or `get_plugin_example`. Do not omit it — the tool's `latestStable` default may not match the version you just pinned.

## Step 3 — Remove a dependency

Read `PklProject`, find the dependency block for the named plugin, and delete it using the Edit tool. Show the diff to the user.

**Dangling import check.** Before confirming the removal, search all `.pkl` files in the project for `import "@<name>/` (and `amends "@<name>/`). If any match is found, warn the user that removing the dependency will leave dangling imports that will cause resolution errors. List the affected files and ask whether to proceed anyway. Do not automatically remove the imports — that is the user's decision.

After deleting from `PklProject`, run `pkl project resolve` (see Step 2c) to update the lockfile.

## Step 4 — Agent install note

This skill manages **schema packages only**. Schema packages provide PKL types for authoring and `formae eval`. They do not install resource plugins on the agent.

If the user actually needs the resource plugin to **run** (i.e., to execute `apply` against real infrastructure), that is an agent-side install that is out of scope for this skill. Point the user to the `formae-plugin-new` skill or to docs.formae.io for agent plugin installation instructions.

---

## CONSTRAINTS

- **Schema deps only.** This skill does not install resource plugins on the agent, does not modify `formae root`, and makes no changes to the agent or running infrastructure.
- **Never silently add an unverified-originator plugin.** If `originatorVerified` is false, surface the originator domain and get explicit user confirmation before adding the dependency.
- **Use `pkl project resolve`** to update `PklProject.deps.json` — `formae project resolve` does not exist (`formae project` only has `init`). Show the command before running it.
- **Never skip the dangling-import check on removal.** Always scan `.pkl` files before confirming a dependency removal.
