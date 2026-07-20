---
name: formae-apply
description: "Use when the user wants to deploy infrastructure, apply a forma file, reconcile a stack, update a stack, or make planned infrastructure changes"
---

# Apply Infrastructure (Reconcile Mode)

Use the `apply_forma` MCP tool in **reconcile** mode to deploy or update infrastructure.

## Targeting an environment (`profile`)

`apply_forma` (and `get_command_status`) hit the formae agent's API directly and take an optional `profile` argument. If the user is working against a specific environment (e.g. `prod`, `staging`), pass that profile name as `profile` on the `apply_forma` call and any `get_command_status` follow-up so it targets that environment — for this session only, without changing global state. If which environment they mean is unclear and `list_profiles` shows more than one, ask first. Never use `use_profile` to "set up" this session — the active profile is global and shared with the user's CLI and any other open sessions. When no profile is named, the active profile is used. Requires formae >= 0.87.0.

## How Reconcile Works

Reconcile guarantees the target infrastructure matches the forma file exactly:
- Resources in the file but not deployed are **created**
- Deployed resources not in the file are **destroyed**
- Differences between file and deployed state are **updated**

This is the standard mode for planned deployments.

## Workflow

1. Confirm the forma file path with the user
2. **Always simulate first**: call `apply_forma` with `mode: reconcile`, `simulate: true`
3. Present the simulation results clearly:
   - Resources to be created
   - Resources to be updated (show what changes)
   - Resources to be destroyed
4. **Ask for explicit confirmation** before proceeding
5. If confirmed: call `apply_forma` with `mode: reconcile`, `simulate: false`
6. The command runs asynchronously. Poll `get_command_status` to monitor progress:
   - **Wait 5 seconds between polls** (`sleep 5`). Do NOT poll in a tight loop.
   - **Only report state transitions** — do NOT print anything unless a resource changed status since the last poll (e.g., in_progress → completed, in_progress → failed). Silently poll until something changes.
   - When reporting, summarize what changed (e.g., "3 resources created, VPC now deploying") rather than dumping the full JSON.
7. Report the final result

## Error Recovery

If `get_command_status` returns a **failed** state:
1. Report which resources failed and the error messages clearly.
2. Do NOT automatically retry — ask the user how to proceed.
3. Common options to offer:
   - **Fix and retry**: address the root cause (permissions, quotas, naming conflicts) then re-run the workflow from simulation.
   - **Roll back to a previous state**: reconcile with the previous forma file to converge infrastructure back toward the prior desired state. Reconcile cannot restore destroyed data or undo billable side effects from the partial deployment — flag this caveat to the user before proceeding.
   - **Investigate**: use `get_command_status` details or provider logs to diagnose further.

## Force Flag

If the simulation reports drift (out-of-band changes detected), the apply may be rejected. The user can choose to:
- **Investigate**: Use the `formae-fix-code-drift` skill to understand the changes
- **Force**: Set `force: true` to overwrite the drift

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER apply without user confirmation
- For targeted urgent fixes, use the `formae-patch` skill instead
