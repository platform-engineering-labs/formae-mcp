---
name: formae-resources
description: "Use when the user asks about deployed infrastructure, what resources exist, resource counts, or wants to find specific resources by type, stack, label, or management status"
---

# Query Infrastructure Resources

Use the `list_resources` MCP tool to query the formae agent for infrastructure resources.

## Targeting an environment (`profile`)

`list_resources` (and `get_agent_stats`) hit the formae agent's API directly and take an optional `profile` argument. If the user is working against a specific environment (e.g. `prod`, `staging`), pass that profile name as `profile` on each query in this flow so it targets that environment — for this session only, without changing global state. If which environment they mean is unclear and `list_profiles` shows more than one, ask first. Never use `use_profile` to "set up" this session — the active profile is global and shared with the user's CLI and any other open sessions. When no profile is named, the active profile is used. Requires formae >= 0.87.0.

## Important: Avoid Unbounded Queries

The resources endpoint returns ALL matching resources with full properties. An empty query on a large environment can return hundreds of thousands of characters, overwhelming the context window.

**Always narrow the query** using at least one filter. If the user asks a broad question like "what resources do we have?", start with `get_agent_stats` for counts, then drill down.

## Workflow

1. For broad questions ("what do we have?"): call `get_agent_stats` first to get resource counts by provider
2. For specific questions: translate to a Bluge query with at least one filter
3. Call `list_resources` with the narrowed query
4. Present results grouped logically (by stack, type, or target as appropriate)

## Query Examples

| User asks... | Approach |
|---|---|
| "What resources do we have?" | Use `get_agent_stats` for overview, then drill down |
| "How many S3 buckets?" | `type:AWS::S3::Bucket` |
| "What's in production?" | `stack:production` |
| "Show unmanaged resources" | `managed:false` |
| "S3 buckets in staging" | `type:AWS::S3::Bucket stack:staging` |
| "Find my-api resources" | `label:my-api` |

Read the `formae://docs/query-syntax` resource for the full query syntax reference.

## Presentation

- Group by stack or type depending on context
- Show resource type, label, and key properties
- Highlight management status (managed vs unmanaged)
- Summarize with counts before showing full details
