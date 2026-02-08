---
name: formae-status
description: "Use when the user asks about running commands, deployment progress, recent operations, command history, or what failed"
---

# Command Status and Monitoring

Use `list_commands` and `get_command_status` MCP tools to monitor formae operations.

## Workflow

1. For an overview: call `list_commands` (optionally with a query)
2. For a specific command: call `get_command_status` with the command ID
3. Present status clearly with resource update progress

## Common Queries

| User asks... | Tool + Query |
|---|---|
| "What's running?" | `list_commands` with `status:in_progress` |
| "Show recent commands" | `list_commands` (no query, defaults to 10 most recent) |
| "What failed?" | `list_commands` with `status:failed` |
| "Show my commands" | `list_commands` with `client:me` |
| "Status of cmd-123" | `get_command_status` with `command_id: cmd-123` |

## Watching a Command

When the user asks to "watch" or "follow" a command:
1. Call `get_command_status` with the command ID
2. If the command is still `in_progress`, report current progress
3. Offer to check again after a short interval
4. Continue until the command reaches a terminal state (`completed`, `failed`, `canceled`)

## Command States

- **pending**: Queued, not yet started
- **in_progress**: Currently executing
- **completed**: Successfully finished
- **failed**: Encountered errors
- **canceled**: Stopped by user
