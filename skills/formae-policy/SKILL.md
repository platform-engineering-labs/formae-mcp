---
name: formae-policy
description: "Use when the user wants to set, remove, or inspect a TTL or auto-reconcile policy on a stack — e.g. 'expire X in 20 minutes', 'reject out-of-band changes on Y', 'auto-reconcile production every 5 minutes', 'remove the TTL on dev', 'what policies are on lifeline?'"
---

# Manage Stack Policies

Use this skill to manage formae stack policies via natural language. Two policy types are supported:

- **TTL** — destroys a stack after a duration (`TTLPolicy`).
- **Auto-reconcile** — periodically reverts out-of-band changes (`AutoReconcilePolicy`).

## Source and installation context

For mode `none`, the user-facing plan contains stack names, policy behavior,
duration, affected resources and outcomes. Treat file paths, snippets, anchors
and edits in the steps below as internal authoring details; ask about policy
choices rather than files. Mode `codebase` retains selected-source reporting.

Pass the chosen `profile` on every policy planner and agent call. Call `get_codebase_context` with the actual harness working directory or reuse the selected context. For `none`, use `prepare_authoring` to retrieve the complete affected stacks into an empty disposable directory, preserving existing policies, targets, references and generators. A maintained project is optional.

Every planner call carries the selected `context` plus explicit `forma_file` inside that project or disposable directory, including `delete_standalone_policy`. Planners return snippets and anchors; the harness edits those files. Scans stay in that one selected root. Always pass that context on later apply/destroy calls. For shared policies affecting multiple stacks, prepare the complete set and review all affected stacks. Never switch global profiles or search the MCP process directory.

Use `formae-apply` for soft reconcile, drift decisions and the final combined confirmation. Suggest the optional message in that confirmation, allowing edit or clear. Keep disposable source and dependencies until outcome/retry inspection is complete, then remove the directory. Older agents without the required capabilities need a maintained complete source project.

## Inline vs standalone — pick the right kind

- **Inline policy** — declared inside a single stack block. Use when the policy is one-off and applies to exactly one stack.
- **Standalone (reusable) policy** — declared at the top level of `forma { }` and referenced from one or more stacks via `PolicyResolvable`. Use when the same policy governs multiple stacks (e.g. a shared 1-hour ephemeral TTL applied to `lifeline`, `dev`, and `staging`).

When the user mentions attaching the same policy to more than one stack, or explicitly says "reusable" or "shared", default to standalone. Otherwise default to inline.

**A stack may hold at most one policy per type.** It cannot carry both an inline TTL and a standalone TTL. The tools enforce this and will refuse with an error naming the conflict; relay that error and offer to detach or remove the conflicting policy rather than working around it.

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
   - `existing_policy_snippet` — only for `update`; use it for the before/after policy comparison, including the source diff in mode `codebase`.
   - `imports_to_add` — add each entry near the top of the file if missing.
5. **Read the file.**
6. **Apply the edit.** For `create` insert the snippet before the anchor line; for `update` replace the lines covered by the anchor range. Add any missing imports near the top of the file. Indent the snippet to match the surrounding context (the snippet is emitted unindented).
7. **Review the change.** In mode `none`, describe the policy's before/after behavior; in mode `codebase`, show the relevant source diff as well.
8. **Prepare the preview.** Source edits alone do not activate a policy. If the user declines, stop. In mode `none`, abandon the unsubmitted preview and clean up its disposable workspace; do not promise it will apply later. In mode `codebase`, explain that the local edit remains unapplied.
9. **Simulate.** Call `apply_forma` with `mode: "reconcile"`, `simulate: true`, `file_path: <returned file_path>`.
10. **Show the simulation, ask for explicit apply confirmation.**
11. **Apply for real.** Call `apply_forma` with `simulate: false`. Then poll `get_command_status` every 5 seconds (with `sleep 5`); only report state transitions.

## Workflow — remove a policy

User says "don't expire lifeline anymore", "stop auto-reconciling production", etc.

Same shape as set, but call `create_inline_policy` with `operation: "remove"`. The tool returns:
- `operation: "remove"` — apply the deletion at the anchor lines.
- `operation: "noop"` — no policy of that type was attached. Tell the user there's nothing to remove and stop.

For `remove`, delete the lines covered by the anchor range. Use `existing_policy_snippet` to describe the policy being removed; show the source diff only in mode `codebase`.

If `notes` mentions "removed empty policies block", retain that source-cleanup detail internally in mode `none`; explain only the policy removed. In mode `codebase`, it can accompany the relevant source diff.

## Workflow — create a standalone policy

User says "create a 1-hour ephemeral policy and attach it to lifeline and dev".

1. **Parse the intent.** Resolve a label, the policy type, and the duration in seconds. If the user gave no label, propose a descriptive one (`ephemeral-1h`, `nightly-drift`) and confirm it.
2. **Ask which stacks to attach to**, if the user has not said. Creating a standalone attaches it to nothing and changes no infrastructure by itself.
3. **Plan the declaration.** Call `create_standalone_policy` with `label`, `policy_type` and the duration field. It returns `file_path`, `pkl_snippet`, `insertion_anchor_start`/`_end` (equal — insert BEFORE that line, which is the forma block's closing brace), and `imports_to_add`.
   - `operation: "noop"` means a standalone with that label already exists. Say so and stop; updating in place is not supported (delete and recreate).
   - An ambiguity error means several files tie for "most stacks". Present the candidates, ask the user once which to use, and pass it as `forma_file` for the rest of the session.
4. **Read the file, apply the edit** with Edit, adding any missing imports near the top. Indent the snippet to match its surroundings.
5. **Attach to each named stack.** For each, run the attach workflow below through its edit step. Collect the set of files touched.
6. **Simulate.** Call `apply_forma` with `mode: "reconcile"`, `simulate: true` on the file carrying the declaration. If attach targets live in other files, simulate each of those too.
7. **Show the simulation, get explicit confirmation, apply for real**, then poll `get_command_status` every 5 seconds and report only state transitions.

## Workflow — attach a standalone policy to a stack

User says "attach ephemeral-1h to staging".

1. Call `attach_standalone_policy` with `stack` and `policy_label`. It returns `file_path`, `pkl_snippet`, `insertion_anchor_start`/`_end` (equal — insert BEFORE that line), `imports_to_add`, `notes`.
   - `operation: "noop"` means it is already attached. Say so and stop.
   - An error naming an **inline** policy of the same type: the stack already has one. Offer to remove it first (the remove workflow above), then retry.
   - An error naming a **standalone** of the same type: offer to detach that one first, then retry.
   - An error saying the policy is unknown to the agent: the declaration exists in source but has not been applied. Apply the declaring file first.
2. Read the file and apply the edit with Edit. Describe the policy attachment; show the source diff only in mode `codebase`.
3. Simulate with `apply_forma` reconcile, confirm, apply, poll.

## Workflow — detach a standalone policy from a stack

User says "detach ephemeral-1h from lifeline".

1. Call `detach_standalone_policy` with `stack` and `policy_label`. It returns `source_anchor_start`/`_end` — the lines to **delete** — plus `existing_resolvable_snippet` for the diff.
   - `operation: "noop"` means it was not attached. Say so and stop.
   - If `notes` mentions "removed empty policies block", keep that source-cleanup detail internal in mode `none`; show it with the relevant source diff in mode `codebase`.
2. Delete the line range with Edit. Describe the policy detachment; show the source diff only in mode `codebase`.
3. Simulate with `apply_forma` reconcile, confirm, apply, poll.

Detaching does not delete the policy. It stays declared and stays attached to any other stacks.

## Workflow — delete a standalone policy

User says "delete the ephemeral-1h policy".

1. Call `delete_standalone_policy` with `label`.
   - If it errors listing attached stacks, the policy is still in use. Tell the user which stacks, and offer to detach it from each (and apply those changes) before retrying. Do not try to force it.
2. On success the tool returns `file_path`, `source_anchor_start`/`_end`, `existing_policy_snippet`, and `destroy_forma_pkl`.
3. **Show the plan and get confirmation before touching anything.**
4. **Delete the source declaration first** with Edit. This ordering is deliberate: if a reconcile lands between the edit and the destroy, the agent sees no policy in any forma and does nothing. Reversed, a reconcile in between would recreate the policy.
   - If `notes` warns about a `local` binding, also remove the bare reference inside `forma { }` and any `<binding>.res` entries, or the file will not evaluate.
5. **Write `destroy_forma_pkl` verbatim to a temporary file inside the selected workspace**, so the existing PklProject/dependencies and source context remain available.
6. Call `destroy_forma` with `file_path: <temp>`, `simulate: true`. Show the result, get explicit confirmation, then call it with `simulate: false` and poll.
7. Delete the temp file.

If the destroy returns a `Skip` operation with `ReferencingStacks`, someone attached the policy between the pre-check and the destroy. Say the policy still exists and name the attaching stacks. In mode `codebase`, also explain that the source has already been edited and needs reconciliation with this outcome. In mode `none`, keep the disposable edit internal; never report successful deletion.

**Version gating.** The standalone-policy tools require formae ≥ 0.82.0, and the auto-reconcile policy type requires formae ≥ 0.88.0. On an older local formae the tool refuses with a `requires formae >= X.Y.Z` message — relay it and suggest upgrading, or fall back to an inline TTL policy where that fits.

## Workflow — show policies on a stack

User asks "what policies are on lifeline?".

1. Call `list_stacks`, locate the stack, surface its inline `Policies`.
2. Call `list_policies`, filter to entries whose `AttachedStacks` includes the target stack label.
3. Present both, and label which is which — inline policies belong to that stack alone; a standalone attached to it may also govern other stacks (its `AttachedStacks` shows them all). This distinction matters: removing an inline policy affects one stack, while deleting a standalone affects every stack it is attached to. No file edits, no apply.

## Important

- NEVER use `pkl eval` — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions.
- NEVER apply without simulating first.
- NEVER apply without explicit user confirmation.
- Pkl is the code interface in both maintained and disposable workspaces. Central accepted desired intent is recorded by real reconcile; an edit or preview alone is not acceptance.
- When a planner returns ambiguous source candidates, resolve within the selected
  source. In mode `codebase`, ask which candidate file to edit if needed. In mode
  `none`, reprepare the complete requested stack or ask which infrastructure
  stack/policy the user means; never make them choose a temporary file.
- A stack holds at most one policy per type. Never work around a conflict error by editing the PKL directly — resolve it by removing or detaching the conflicting policy.
- Standalone policies are created and deleted, never updated in place. To change one, delete it and recreate it, or convert the stack to an inline policy.
- If a tool reports the selected schema is too old for policies, mode `codebase` requires a separate schema-upgrade decision. In mode `none`, verify the prepared source matches the connected agent and use its supported schema; if that agent lacks the capability or its schema is unavailable, explain the policy capability limitation and stop. Never guess a newer schema or imply upgrading a schema upgrades the agent.
