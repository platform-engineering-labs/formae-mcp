---
name: formae-apply
description: "Deploy and reconcile infrastructure using forma files. Simulates changes before applying, monitors async deployment progress, and handles drift detection. Use when the user says 'deploy infrastructure', 'apply forma', 'reconcile stack', 'infrastructure changes', 'deploy changes', 'apply forma file', 'update stack'."
---

# Apply Infrastructure (Reconcile Mode)

Use the `apply_forma` MCP tool in **reconcile** mode to deploy or update infrastructure. Reconcile ensures deployed state matches the forma file exactly — creating, updating, or destroying resources as needed.

## Workflow

1. Confirm the forma file path with the user
2. **Validate the forma file**: run `formae eval --output-consumer machine <file>` to check for syntax or resolution errors before proceeding. If evaluation fails, report the error and stop.
3. **Always simulate first**: call `apply_forma` with `mode: reconcile`, `simulate: true`
4. Present the simulation results clearly:
   - Resources to be created
   - Resources to be updated (show what changes)
   - Resources to be destroyed
5. **Ask for explicit confirmation** before proceeding
6. If confirmed: call `apply_forma` with `mode: reconcile`, `simulate: false`
7. The command runs asynchronously. Poll `get_command_status` to monitor progress:
   - **Wait 5 seconds between polls** (`sleep 5`). Do NOT poll in a tight loop.
   - **Only report state transitions** — do NOT print anything unless a resource changed status since the last poll (e.g., in_progress → completed, in_progress → failed). Silently poll until something changes.
   - When reporting, summarize what changed (e.g., "3 resources created, VPC now deploying") rather than dumping the full JSON.
8. Report the final result

## Error Recovery

If `get_command_status` returns a **failed** state:
1. Report which resources failed and the error messages clearly.
2. Do NOT automatically retry — ask the user how to proceed.
3. Common options to offer:
   - **Fix and retry**: address the root cause (permissions, quotas, naming conflicts) then re-run the workflow from simulation.
   - **Partial rollback**: if some resources succeeded, offer to run a new reconcile with the previous forma file to revert.
   - **Investigate**: use `get_command_status` details or provider logs to diagnose further.

## Force Flag

If the simulation reports drift (out-of-band changes detected), the apply may be rejected. The user can choose to:
- **Investigate**: Use `/formae-fix-code-drift` to understand the changes
- **Force**: Set `force: true` to overwrite the drift

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER apply without user confirmation
- For targeted urgent fixes, use `/formae-patch` instead
