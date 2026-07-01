---
name: formae-discover
description: "Use when the user wants to find unmanaged resources in their cloud accounts, see what's not managed by formae, or run a discovery scan"
---

# Discover Unmanaged Resources

Find resources in cloud accounts that aren't managed by formae yet.

## Targeting an environment (`profile`)

`get_agent_stats` and `list_resources` hit the formae agent's API directly and take an optional `profile` argument. If the user is working against a specific environment (e.g. `prod`, `staging`), pass that profile name as `profile` on each call in this flow so it targets that environment — for this session only, without changing global state. If which environment they mean is unclear and `list_profiles` shows more than one, ask first. Never use `use_profile` to "set up" this session — the active profile is global and shared with the user's CLI and any other open sessions. When no profile is named, the active profile is used. Requires formae >= 0.87.0.

## Workflow

1. Call `get_agent_stats` first to get an overview of unmanaged resource counts by provider
2. Based on the counts, use targeted `list_resources` queries with specific type filters to drill down (e.g., `managed:false type:AWS::S3::Bucket`). **Never** call `list_resources` with just `managed:false` — on real accounts this returns too much data and will overflow the context window.
3. Present results grouped by resource type, showing:
   - Resource type and label
   - Key properties
   - Which target/account they belong to
4. Ask the user if they want to bring any resources under management
5. If yes, use the `formae-import` skill to start the import workflow

## Forcing a Fresh Discovery

If the user wants the latest view, call `force_discover` to trigger an immediate discovery scan before querying. Note that discovery runs asynchronously — wait a moment before querying results.

## Presentation

- Start with a high-level summary of counts by type
- Only show full details for types the user is interested in
- Highlight resources that look like they belong to existing stacks
