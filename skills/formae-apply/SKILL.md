---
name: formae-apply
description: "Use when the user wants to deploy infrastructure, apply a forma file, reconcile a stack, or make planned infrastructure changes"
---

# Apply Infrastructure (Reconcile Mode)

Use the `apply_forma` MCP tool in **reconcile** mode to deploy or update infrastructure.

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
6. The command runs asynchronously. Poll `get_command_status` to monitor progress, but **wait 5 seconds between polls** to avoid burning context window. Do NOT poll in a tight loop. Use `sleep 5` between calls.
7. Report the final result

## Force Flag

If the simulation reports drift (out-of-band changes detected), the apply may be rejected. The user can choose to:
- **Investigate**: Use `/formae-fix-code-drift` to understand the changes
- **Force**: Set `force: true` to overwrite the drift

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER apply without user confirmation
- For targeted urgent fixes, use `/formae-patch` instead
