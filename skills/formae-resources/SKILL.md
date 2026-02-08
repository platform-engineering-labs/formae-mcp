---
name: formae-resources
description: "Use when the user asks about deployed infrastructure, what resources exist, resource counts, or wants to find specific resources by type, stack, label, or management status"
---

# Query Infrastructure Resources

Use the `list_resources` MCP tool to query the formae agent for infrastructure resources.

## Workflow

1. Translate the user's natural language request into a Bluge query
2. Call `list_resources` with the query
3. Present results grouped logically (by stack, type, or target as appropriate)

## Query Examples

| User asks... | Query |
|---|---|
| "What resources do we have?" | _(empty)_ |
| "Show me all S3 buckets" | `type:AWS::S3::Bucket` |
| "What's in production?" | `stack:production` |
| "Show unmanaged resources" | `managed:false` |
| "S3 buckets in staging" | `type:AWS::S3::Bucket stack:staging` |
| "Find my-api resources" | `label:my-api` |

Read the `formae://docs/query-syntax` resource for the full query syntax reference.

## Presentation

- Group by stack or type depending on context
- Show resource type, label, and key properties
- Highlight management status (managed vs unmanaged)
- For large result sets, summarize with counts before showing details
