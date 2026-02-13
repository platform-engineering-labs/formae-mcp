---
name: formae-destroy
description: "Use when the user wants to destroy, delete, or tear down infrastructure resources, stacks, or environments"
---

# Destroy Infrastructure

Use the `destroy_forma` MCP tool to remove infrastructure resources.

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
6. Poll `get_command_status` to monitor progress, but **wait 5 seconds between polls** to avoid burning context window. Do NOT poll in a tight loop. Use `sleep 5` between calls.
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
