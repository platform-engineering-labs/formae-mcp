---
name: formae-stacks
description: "Use when the user asks about their infrastructure stacks, how infrastructure is organized, or needs a stack overview with resource counts"
---

# List Infrastructure Stacks

Use the `list_stacks` MCP tool to retrieve all stacks from the formae agent.

## Workflow

1. Call `list_stacks` (no parameters needed)
2. Present stacks with their label, description, and resource count
3. If the user wants to drill into a specific stack, use `list_resources` with `stack:<name>`

## What is a Stack?

A stack is a logical grouping of resources with referential integrity. Destroying one resource in a stack may cascade to dependent resources. The default stack is "default". Unmanaged resources discovered by the agent live on the "unmanaged" stack.
