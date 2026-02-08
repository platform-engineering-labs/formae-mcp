---
name: formae-targets
description: "Use when the user asks about their cloud targets, configured regions, provider accounts, or which cloud accounts are set up"
---

# Query Cloud Targets

Use the `list_targets` MCP tool to query configured cloud targets (accounts/regions).

## Workflow

1. Translate the user's request into a query
2. Call `list_targets` with the query
3. Present targets grouped by namespace (cloud provider)

## Query Examples

| User asks... | Query |
|---|---|
| "What targets are configured?" | _(empty)_ |
| "Show AWS targets" | `namespace:AWS` |
| "Which targets have discovery?" | `discoverable:true` |

## What is a Target?

A target represents a cloud account or region where resources are deployed. Each target has a namespace (e.g., AWS, Azure) and configuration (region, credentials).
