---
name: formae-policy
description: "Use when the user wants to set, remove, or inspect a TTL or auto-reconcile policy on a stack — e.g. 'expire X in 20 minutes', 'reject out-of-band changes on Y', 'auto-reconcile production every 5 minutes', 'remove the TTL on dev', 'what policies are on lifeline?'"
---

# Manage Stack Policies

Use this skill to manage formae stack policies via natural language. Two policy types are supported:

- **TTL** — destroys a stack after a duration (`TTLPolicy`).
- **Auto-reconcile** — periodically reverts out-of-band changes (`AutoReconcilePolicy`).

## Inline vs standalone — pick the right kind

- **Inline policy** — declared inside a single stack block. Use when the policy is one-off and applies to exactly one stack.
- **Standalone (reusable) policy** — declared at the top level of `forma { }` and referenced from one or more stacks via `PolicyResolvable`. Use when the same policy governs multiple stacks (e.g. a shared 1-hour ephemeral TTL applied to `lifeline`, `dev`, and `staging`).

When the user mentions attaching the same policy to more than one stack, or explicitly says "reusable" or "shared", default to standalone. Otherwise default to inline.

A stack should not carry both an inline and a standalone policy of the same type. The tooling does NOT enforce this for you — before adding a policy, check existing policies with `list_policies` and `list_stacks` and make sure you're not creating a duplicate of the same type on the stack.

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

## Standalone (reusable) policies

> **Availability:** The `create_standalone_policy` / `attach_standalone_policy` / `detach_standalone_policy` / `delete_standalone_policy` MCP tools described below are part of a planned feature and may not be present on the connected MCP server. **Before using any of them, confirm the tool is actually available.** If these tools are NOT available, do standalone policies manually instead (see "Manual fallback" below) — and only if the connected agent supports standalone policies at all; if unsure, tell the user standalone-policy support may not be available yet and offer an inline policy instead.

A standalone policy is declared once at the top level of the main forma file (`forma { }` block, not inside any stack) and attached to stacks via `PolicyResolvable` references.

### PKL shape

```pkl
forma {
  // Standalone policy declaration (top level, outside any stack)
  new formae.TTLPolicy {
    label = "ephemeral-1h"
    ttl = 1.h
    onDependents = "abort"
  }

  // (Auto-reconcile variant)
  new formae.AutoReconcilePolicy {
    label = "reconcile-5m"
    interval = 5.min
  }

  // Stacks reference it via PolicyResolvable
  new formae.Stack {
    label = "lifeline"
    // ...resources...
    policies = new Listing {
      new formae.PolicyResolvable { label = "ephemeral-1h" }
    }
  }
}
```

### Manual fallback (no standalone tools)

When the standalone MCP tools are NOT available, you can still create and attach a standalone policy by editing the PKL file directly:

1. **Read the main forma file** — identify the file that contains the `forma { }` block with the most stacks.
2. **Insert the policy declaration** — add the standalone policy at the top level inside the `forma { }` block (not inside any stack), using the PKL shape shown above (e.g. `new formae.TTLPolicy { label=...; ttl=... }` or `new formae.AutoReconcilePolicy { label=...; interval=... }`).
3. **Add a `PolicyResolvable` reference** — for each target stack, add (or extend) a `policies = new Listing { ... }` block containing `new formae.PolicyResolvable { label = "<policy-label>" }`.
4. **Add any missing imports** near the top of the file.
5. **Show the diff** to the user.
6. **Simulate first:** call `apply_forma` with `mode: "reconcile"`, `simulate: true`, `force: true`, `file_path: <file>`. Show the simulation result and ask for explicit confirmation.
7. **Apply for real:** call `apply_forma` with `simulate: false`. Poll `get_command_status` every 5 seconds; only report state transitions.

> **Note:** this manual path still depends on the connected formae agent supporting standalone policies. If the agent does not recognise them, the apply will fail. In that case, fall back to an inline policy instead.

### Workflow — create a standalone policy (with optional attach)

_Use this workflow when the standalone MCP tools are available (see Availability note above)._

User says something like "create a 1-hour ephemeral policy and attach it to lifeline and dev".

1. **Parse the intent.** Resolve policy fields and any stacks to attach to.
2. **Call `create_standalone_policy`** with `label`, `policy_type`, duration fields, and optionally a `forma_file` override. The tool identifies the main forma file (the PKL file with the most stacks) and returns `file_path`, `pkl_snippet`, `insertion_anchor`, and `imports_to_add`. If the tool returns an ambiguity error, present the candidate files to the user and ask which to use (cache the choice for the session).
3. **Read the file**, apply the edit (insert the snippet before the closing `}` of the `forma { }` block), add any missing imports.
4. **For each stack to attach:** call `attach_standalone_policy(stack, policy_label)`. The tool returns a `PolicyResolvable` snippet and an insertion anchor inside the stack's `policies` block (or creates the block if absent). Apply the edit. If the tool returns `noop`, inform the user and continue.
5. **Show the diff.** Ask whether to apply.
6. **Simulate.** Call `apply_forma` with `mode: "reconcile"`, `simulate: true`, `force: true`. If edits span multiple files, simulate each file.
7. **Confirm and apply for real.** Poll `get_command_status` every 5 seconds; report state transitions.

### Workflow — attach a standalone policy to a stack

_Use this workflow when the standalone MCP tools are available (see Availability note above)._

User says "attach ephemeral-1h to staging".

1. Call `attach_standalone_policy(stack, policy_label)`. On conflict (inline policy of same type exists), the tool errors — surface the message and suggest removing the inline policy first. On `noop`, tell the user it's already attached.
2. Read the target file, apply the edit, show the diff.
3. Simulate → confirm → apply → poll.

### Workflow — detach a standalone policy from a stack

_Use this workflow when the standalone MCP tools are available (see Availability note above)._

User says "detach ephemeral-1h from lifeline".

1. Call `detach_standalone_policy(stack, policy_label)`. Returns the line range to delete. On `noop` (not attached), inform the user and stop.
2. Read the file, delete the lines. If `notes` mentions "removed empty policies block", explain to the user that the wrapper block was also removed.
3. Show the diff. Simulate with `apply_forma reconcile`. Confirm → apply → poll.

### Workflow — delete a standalone policy

_Use this workflow when the standalone MCP tools are available (see Availability note above)._

User says "delete the ephemeral-1h policy".

1. Call `delete_standalone_policy(label)`. If the policy is still attached to stacks the tool refuses and lists the offending stacks — surface this and suggest detaching first.
2. On success the tool returns: `file_path` + `source_anchor` (lines to delete), `existing_policy_snippet` (show as "before"), and `destroy_forma_pkl` (a complete PKL forma for the destroy step).
3. **Remove the source declaration first:** read the file, delete the lines at `source_anchor`, show the diff.
4. Write `destroy_forma_pkl` to a temp file. Call `destroy_forma(simulate=true)`, show the simulation, confirm.
5. Call `destroy_forma(simulate=false)`, poll. Clean up the temp file.
6. If the agent's destroy returns a `Skip` with `ReferencingStacks` (race condition), surface this clearly: the source PKL has been edited but the policy still exists in the agent; name the attaching stacks so the user can detach and retry.

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
