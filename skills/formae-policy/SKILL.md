---
name: formae-policy
description: "Use when the user wants to set, remove, or inspect a TTL or auto-reconcile policy on a stack — e.g. 'expire X in 20 minutes', 'reject out-of-band changes on Y', 'auto-reconcile production every 5 minutes', 'remove the TTL on dev', 'what policies are on lifeline?'"
---

# Manage Stack Policies

Use this skill to manage formae stack policies via natural language. Two policy types are supported:

- **TTL** — destroys a stack after a duration (`TTLPolicy`).
- **Auto-reconcile** — periodically reverts out-of-band changes (`AutoReconcilePolicy`).

Reusable / standalone policies are out of scope for this skill (handled separately).

## Defaults

- TTL `onDependents` defaults to `"abort"`. Override with `"cascade"` if the user says "cascade", "destroy dependents too", or similar.
- Auto-reconcile `interval` defaults to **5 minutes** (`300` seconds) when the user does not specify one.

Always state the chosen defaults to the user before applying.

## Workflow — set or update a policy

User says something like "expire lifeline in 20 minutes" or "auto-reconcile production every 10 minutes".

1. **Parse the intent.** Resolve the stack label and the policy parameters. Convert the duration to seconds (`20m` → `1200`, `4h` → `14400`, `1d` → `86400`).
2. **Verify the stack exists.** Call `list_stacks` and find a matching label. If no match, present the available labels and ask the user which one they meant.
3. **Show the user what's about to change.** Briefly state: stack name, policy type, value, default fields filled in. Example: *"I'll attach a TTL of 20 minutes (`onDependents = "abort"` by default) to the `lifeline` stack."*
4. **Plan the file edit.** Call `create_inline_policy` with `operation: "set"`, the resolved `policy_type`, and the relevant duration field (`ttl_seconds` or `interval_seconds`). The tool returns:
   - `file_path` — the PKL file declaring the stack.
   - `operation` — `"create"` or `"update"`.
   - `pkl_snippet` — the text to insert.
   - `insertion_anchor_start` / `insertion_anchor_end` — the line range; for `create` they are equal (insert before that line); for `update` they cover the existing block (replace those lines).
   - `existing_policy_snippet` — only for `update`; show this as the "before" in the diff.
   - `imports_to_add` — add each entry near the top of the file if missing.
5. **Read the file** with the Read tool.
6. **Apply the edit** with the Edit tool. For `create` insert the snippet before the anchor line; for `update` replace the lines covered by the anchor range. Add any missing imports near the top of the file. Indent the snippet to match the surrounding context (the snippet is emitted unindented).
7. **Show the diff** to the user.
8. **Ask whether to apply to infrastructure.** Default phrasing: *"Apply this change with `reconcile` (simulate first)?"* If the user declines, stop — the file edit stands and the policy will activate on the next manual apply.
9. **Simulate.** Call `apply_forma` with `mode: "reconcile"`, `simulate: true`, `force: true`, `file_path: <returned file_path>`.
10. **Show the simulation, ask for explicit apply confirmation.**
11. **Apply for real.** Call `apply_forma` with `simulate: false`. Then poll `get_command_status` every 5 seconds (with `sleep 5`); only report state transitions.

## Workflow — remove a policy

User says "don't expire lifeline anymore", "stop auto-reconciling production", etc.

Same shape as set, but call `create_inline_policy` with `operation: "remove"`. The tool returns:
- `operation: "remove"` — apply the deletion at the anchor lines.
- `operation: "noop"` — no policy of that type was attached. Tell the user there's nothing to remove and stop.

For `remove`, the Edit tool deletes the lines covered by the anchor range. The `existing_policy_snippet` shows what's being removed; surface it in the diff.

If `notes` mentions "removed empty policies block", explain to the user that the stack's `policies = new Listing { ... }` wrapper was also removed because that was the last policy.

## Workflow — show policies on a stack

User asks "what policies are on lifeline?".

1. Call `list_stacks`, locate the stack, surface its inline `Policies`.
2. Call `list_policies`, filter to entries whose `AttachedStacks` includes the target stack label.
3. Present both inline and standalone-attached policies. No file edits, no apply.

## Important

- NEVER use `pkl eval` — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions.
- NEVER apply without simulating first.
- NEVER apply without explicit user confirmation.
- The user's PKL file is the source of truth — always edit the file, never bypass it by going directly to the agent.
- When the tool returns multiple candidate files (ambiguous stack), present the list to the user and ask which file to edit. Do not guess.
