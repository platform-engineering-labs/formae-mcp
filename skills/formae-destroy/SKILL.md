---
name: formae-destroy
description: "Use when the user wants to destroy, delete, or tear down infrastructure resources, stacks, or environments"
---

# Destroy Infrastructure

Use the `destroy_forma` MCP tool to remove infrastructure resources.

## Targeting an environment (`profile`)

`destroy_forma` (and `get_command_status`) hit the formae agent's API directly and take an optional `profile` argument. If the user is working against a specific environment (e.g. `prod`, `staging`), pass that profile name as `profile` on the `destroy_forma` call and any `get_command_status` follow-up so it targets that environment — for this session only, without changing global state. Destroying the wrong environment is especially damaging, so be strict here. If which environment they mean is unclear and `list_profiles` shows more than one, ask first. Never use `use_profile` to "set up" this session — the active profile is global and shared with the user's CLI and any other open sessions. When no profile is named, the active profile is used. Requires formae >= 0.87.0.

## Destroy Modes

Destruction can be specified two ways (mutually exclusive):
- **By file**: Destroys all resources declared in a forma file
- **By query**: Destroys resources matching a query

## Workflow

1. Clarify what the user wants to destroy
2. **Always simulate first**: call `destroy_forma` with `simulate: true`
3. Present what will be destroyed — clearly and completely
4. **Ask for explicit confirmation** — destruction is irreversible
5. If confirmed: call `destroy_forma` with `simulate: false`
6. Poll `get_command_status` to monitor progress:
   - **Wait 5 seconds between polls** (`sleep 5`). Do NOT poll in a tight loop.
   - **Only report state transitions** — do NOT print anything unless a resource changed status since the last poll (e.g., in_progress → completed, in_progress → failed). Silently poll until something changes.
   - When reporting, summarize what changed (e.g., "3 resources deleted, VPC now destroying") rather than dumping the full JSON.
7. Report results

## Common Patterns

| User wants to... | Approach |
|---|---|
| Tear down a stack | `query: "stack:staging"` |
| Remove specific resources | `query: "type:AWS::S3::Bucket label:temp-data"` |
| Destroy what's in a file | `file_path: "/path/to/forma.pkl"` |

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER destroy without explicit user confirmation
- Destruction is **irreversible**
- For large destroy operations, consider destroying one stack at a time
