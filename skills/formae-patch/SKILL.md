---
name: formae-patch
description: "Use when the user needs to make an urgent targeted infrastructure change, hotfix, incident response fix, or patch without a full reconcile"
---

# Patch Infrastructure (Patch Mode)

Use the `apply_forma` MCP tool in **patch** mode only for an explicit patch-mode request or stated incident/hotfix intent requiring a temporary intervention. For ordinary updates, including one label, hand off to `formae-apply` for a complete-stack soft reconcile. Do not infer an incident from a small change, the word "quick", a `.patch.pkl` filename or existing drift.

## Targeting an environment (`profile`)

`apply_forma` (and `get_command_status`) hit the formae agent's API directly and take an optional `profile` argument. If the user is working against a specific environment (e.g. `prod`, `staging`), pass that profile name as `profile` on the `apply_forma` call and any `get_command_status` follow-up so it targets that environment — for this session only, without changing global state. Patches are usually incident/hotfix work against one environment. If which environment they mean is unclear and `list_profiles` shows more than one, ask first. Never use `use_profile` to "set up" this session — the active profile is global and shared with the user's CLI and any other open sessions. When no profile is named, the active profile is used. Requires formae >= 0.87.0.

## How Patch Works

Patch only applies the changes explicitly specified in the forma file. Other resources are untouched. Use this for:
- Incident response (scaling up during traffic spikes)
- Urgent security fixes
- An explicit request to use patch mode and defer reconciliation

For ordinary configuration changes, prepare the complete affected stack and use reconcile without force. Never choose patch to avoid resolving drift or because only a partial file is currently available. Keep the selected source context on both preview and real submission. Explain the temporary nature of a patch in its confirmation; do not automatically absorb it afterward.

## Source context

Reuse the selected `get_codebase_context` result. For `codebase`, keep the incident snippet inside the selected project and carry its binding context. For `none`, call `prepare_authoring` for the affected stack in a fresh `~/.formae-ai/scratch/<operation-id>/` directory, then author a separate minimal patch Pkl file inside it using its schema dependencies. Carry the returned `context` on apply. This workspace is temporary; never register it. Keep it until outcome/retry inspection is complete, then remove it.

## Workflow

In mode `none`, present the targeted infrastructure change and its actual
outcome. Temporary files, source diffs and cleanup stay internal; use the
launcher-selected formae executable from MCP initialization for local CLI work.

1. Help the user identify the resource(s) to modify
2. Create or locate a minimal forma file with only the targeted change
3. **Always simulate first**: call `apply_forma` with `mode: patch`, `simulate: true`
4. Show exactly what will change and suggest a factual optional `message`; the user can edit it or clear it with `message: ""` in the ordinary confirmation
5. **Ask for explicit confirmation**
6. If confirmed: call `apply_forma` with `mode: patch`, `simulate: false`
7. Poll `get_command_status` to monitor progress:
   - **Wait 5 seconds between polls** (`sleep 5`). Do NOT poll in a tight loop.
   - **Only report state transitions** — do NOT print anything unless a resource changed status since the last poll. Silently poll until something changes.
   - When reporting, summarize what changed rather than dumping the full JSON.
8. Report results

## Post-Patch Reminder

After a successful patch, always remind the user:

> This change was made through formae as a temporary patch. When reviewing this temporary intervention,
> decide whether to keep it as the stack's desired state or revert it.

Follow `formae-fix-code-drift` for that later decision. A patch is not a change
made outside formae; use its recorded origin when explaining it.

Do not automatically absorb the patch or update maintained desired source to make it permanent. Resolution controls apply only to soft reconcile, never patch.

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER apply without user confirmation
- Patches are for explicit temporary interventions. For planned changes, use the `formae-apply` skill
