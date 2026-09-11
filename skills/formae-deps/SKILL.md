---
name: formae-deps
description: "Use when the user wants to add or remove a plugin schema dependency in an existing formae project's PklProject — e.g. 'add the grafana plugin', 'I need cloudflare DNS', 'drop the azure dependency'."
---

# Manage Formae Schema Dependencies

Use this skill to add or remove plugin schema dependencies in an existing formae project's `PklProject` file. This is **authoring-only** — schema packages provide PKL types and IDE completion. They do not install resource plugins on the agent and do not make infrastructure changes.

## Step 0 — Check the plugin is available at all

Call `list_agent_plugins` with an explicit `profile`. It reports the installation's plugin set and whether the connection is hosted formae or a self-hosted agent.

On **hosted formae**, refuse to add a schema dependency for a plugin the installation does not report. The types would evaluate locally and the apply would then fail on the agent, which is a worse outcome than the refusal: say the plugin is not reported as installed on this installation, that hosted formae cannot install plugins on demand, and offer what the available set can do instead. For a plugin it does report, take the version from the schema coordinate the tool names rather than from the hub's latest stable, so the project pins what the installation actually runs.

On a **self-hosted agent**, this is advisory only. Adding a schema dependency for a plugin the agent does not have yet is legitimate: authoring and simulate need only the schema, and the user can install the plugin before applying. Mention it, do not block on it.

**If the plugin is present but `list_agent_plugins` names no schema version for it**, stop rather than guessing a package URI. In mode `none`, explain that the installed plugin's schema is unavailable for preparing this resource change; ask about an alternative supported capability only if useful. In mode `codebase`, an experienced user may supply a known compatible coordinate. Never invent a coordinate or replace an installed prerelease with an unrelated stable schema.

## Step 1 — Locate the project's `PklProject`

Carry the source context from `formae-author`. In mode `none`, use the exact
`project_path` returned by `prepare_authoring`; dependency edits and resolution
are internal preparation. Discuss resource capabilities and any material
compatibility issue, not temporary files, diffs or commands. In mode `codebase`,
use the selected project's `PklProject` and show relevant source changes. If no
source is prepared, return to `formae-author` to select context; a missing file
does not opt the user into a maintained project. Never scan arbitrary disk.

## Step 2 — Add a dependency

### 2a — Resolve the package and version

For hosted installations, retain the exact installed schema coordinate from
Step 0; do not replace it with the hub's latest stable version. For self-hosted
catalog lookup, use `get_hub_plugin`. If the name is ambiguous or unknown, use
`search_hub_plugins` and confirm the intended plugin.

For self-hosted catalog lookup, use the resolved plugin record's selected version.

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

Read the file first, then apply the edit. In mode `codebase`, show the relevant
diff. In mode `none`, keep this source edit internal.

### 2c — Resolve the deps lockfile

After editing `PklProject`, update `PklProject.deps.json` with **PKL** (`formae`
has no `project resolve`). Use the companion `pkl` executable beside the resolved
formae binary, or `~/.formae-ai/opt/bin/pkl`, before falling back to PATH. Run from
the selected project directory. Show the command for maintained-codebase work;
in mode `none`, perform this preparation internally:

```
pkl project resolve
```

### 2d — Fetch examples for the newly-added plugin

When pulling examples for the plugin just added, pass the version pinned in `PklProject` (the `@<version>` suffix from the dependency `uri`) as the `version` argument to `list_plugin_examples` or `get_plugin_example`. Do not omit it — the tool's `latestStable` default may not match the version you just pinned.

## Step 3 — Remove a dependency

Read `PklProject`, find the named dependency block, and delete it. Show the diff
in mode `codebase`; keep disposable source edits internal in mode `none`.

**Dangling import check.** Search only the selected source for imports of the
dependency before removal. If still used, explain the affected resources and
resolve the user's intended change before proceeding. In mode `codebase`, list
the affected files as well. Do not silently remove resource declarations or
leave an invalid document merely to complete the dependency edit.

After deleting from `PklProject`, run `pkl project resolve` (see Step 2c) to update the lockfile.

## Step 4 — Agent install note

This skill manages **schema packages only**. Schema packages provide PKL types for authoring and `formae eval`. They do not install resource plugins on the agent.

If the user actually needs the resource plugin to **run** (i.e., to execute `apply` against real infrastructure), that is an agent-side install that is out of scope for this skill. Point the user to the `formae-plugin-new` skill or to docs.formae.io for agent plugin installation instructions.

---

## CONSTRAINTS

- **Schema deps only.** This skill does not install resource plugins on the agent, does not modify `formae root`, and makes no changes to the agent or running infrastructure.
- **Never silently add an unverified-originator plugin.** If `originatorVerified` is false, surface the originator domain and get explicit user confirmation before adding the dependency.
- **Use `pkl project resolve`** to update `PklProject.deps.json` — `formae project resolve` does not exist. Show commands for maintained-codebase work; keep them internal for mode `none`.
- **Never skip the dangling-import check on removal.** Always scan `.pkl` files before confirming a dependency removal.
