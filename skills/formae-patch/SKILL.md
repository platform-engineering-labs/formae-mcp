---
name: formae-patch
description: "Use when the user needs to make an urgent targeted infrastructure change, hotfix, incident response fix, or patch without a full reconcile"
---

# Patch Infrastructure (Patch Mode)

Use the `apply_forma` MCP tool in **patch** mode for urgent targeted changes.

## How Patch Works

Patch only applies the changes explicitly specified in the forma file. Other resources are untouched. Use this for:
- Incident response (scaling up during traffic spikes)
- Urgent security fixes
- Quick configuration changes
- Any situation where a full reconcile is inappropriate

## Workflow

1. Help the user identify the resource(s) to modify
2. Create or locate a minimal forma file with only the targeted change
3. **Always simulate first**: call `apply_forma` with `mode: patch`, `simulate: true`
4. Show exactly what will change
5. **Ask for explicit confirmation**
6. If confirmed: call `apply_forma` with `mode: patch`, `simulate: false`
7. Poll `get_command_status` to monitor progress:
   - **Wait 5 seconds between polls** (`sleep 5`). Do NOT poll in a tight loop.
   - **Only report state transitions** — do NOT print anything unless a resource changed status since the last poll. Silently poll until something changes.
   - When reporting, summarize what changed rather than dumping the full JSON.
8. Report results

## Post-Patch Reminder

After a successful patch, always remind the user:

> This patch will appear as **drift** until you reconcile your IaC code. When the incident is resolved, consider running `/formae-fix-code-drift` to incorporate this change into your codebase.

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER apply without user confirmation
- Patches are for urgency. For planned changes, use `/formae-apply`
