---
name: formae-discover
description: "Use when the user wants to find unmanaged resources in their cloud accounts, see what's not managed by formae, or run a discovery scan"
---

# Discover Unmanaged Resources

Find resources in cloud accounts that aren't managed by formae yet.

## Workflow

1. Call `get_agent_stats` first to get an overview of unmanaged resource counts by provider
2. Based on the counts, use targeted `list_resources` queries with specific type filters to drill down (e.g., `managed:false type:AWS::S3::Bucket`). **Never** call `list_resources` with just `managed:false` — on real accounts this returns too much data and will overflow the context window.
3. Present results grouped by resource type, showing:
   - Resource type and label
   - Key properties
   - Which target/account they belong to
4. Ask the user if they want to bring any resources under management
5. If yes, use `/formae-import` to start the import workflow

## Forcing a Fresh Discovery

If the user wants the latest view, call `force_discover` to trigger an immediate discovery scan before querying. Note that discovery runs asynchronously — wait a moment before querying results.

## Presentation

- Start with a high-level summary of counts by type
- Only show full details for types the user is interested in
- Highlight resources that look like they belong to existing stacks
