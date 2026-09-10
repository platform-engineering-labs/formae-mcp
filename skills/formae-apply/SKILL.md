---
name: formae-apply
description: "Use when the user wants to deploy infrastructure, apply a forma file, reconcile a stack, update a stack, or make planned infrastructure changes"
---

# Apply Infrastructure

Use `apply_forma` in reconcile mode for planned changes. Each explicitly declared stack is a complete resource boundary: omitted resources are deleted, including when a stack contains only generators. Patch is the scoped incident path.

## Source and installation

Pass the chosen `profile` on every agent call; never change the global active profile for a session. Call `get_codebase_context` with the actual harness `working_directory` or reuse the explicit context already selected. Resolve known candidates once and surface missing/corrupt projects.

- `codebase`: carry `context: {mode: "codebase", binding_id: ...}` and use the complete main forma inside that selected project. Every evaluated stack must fit its registered scope. Preserve abstractions and unrelated edits.
- `none`: follow `formae-author` to call `prepare_authoring` in an empty disposable directory. Carry its returned `context` and complete `file_path`; keep PklProject and resolved dependencies. Never use an actual-inventory fragment or partial command delta as full reconcile input. A maintained project is opt-in.
- Older agents without the connected `desired-stack-extraction` and `shared-drift-resolution` capabilities require an existing complete codebase. Do not send new controls or imply local binary version proves server support. Legacy file-based apply remains available.

Pkl remains the code interface in both modes. Use `formae eval --output-schema json --output-consumer machine` for local evaluation. If retaining an evaluated JSON file for exact retry, keep it inside the same selected workspace along with the Pkl source and dependencies. Never edit the evaluated submission after review.

## Workflow

1. Prepare the complete declaration, preserving every existing resource, target, policy, reference and generator in each affected stack unless its change/removal is intended.
2. Call `apply_forma` with the selected `context`, `mode: reconcile`, `simulate: true`, and no force.
3. If rejected for actionable drift, follow `formae-fix-code-drift`: use the initial `ObservationID`, all explicit absorb/revert choices, final composed simulation `ReviewID`, then confirmed real submission with a stable `IdempotencyKey`. Do not pre-edit the source to absorb drift. A stale review requires a fresh review and confirmation.
4. Show the final combined plan, including acceptance, provider work, ordinary changes, deletes, dependencies and warnings. Suggest a factual optional `message` in this same confirmation. The user can edit it or clear with `message: ""`; no extra confirmation round is needed solely for the message.
5. After confirmation, submit the exact reviewed input with `simulate: false`. Carry the final resolution controls when applicable. Retain exact input, message and idempotency key while outcome is uncertain; an idempotent replay uses all of them unchanged.
6. Call `get_command_status` with `wait: true`, and continue waiting if its budget ends before a terminal state. Report actual outcomes, errors, command ID and recorded acceptance separately from provider work. A no-change preview does not record central acceptance.
7. After a terminal resolution, use `get_command_desired_delta` to automatically catch up only the selected maintained codebase via harness source edits. Its `Partial` result is a snippet, never a full reconcile file. Preserve abstractions and unrelated edits, apply recorded deletion guidance, and simulate the complete main forma afterward. Report source conflicts separately from central command success. Never catch up from later unconstrained inventory.
8. For mode `none`, retain the disposable directory through outcome/retry inspection, then remove it. Also clean up abandoned previews after confirming no real request was sent. Never clean up an uncertain submission before retaining what is needed for its exact replay.

A Failed command is still its recorded outcome. Diagnose failures before a new corrective operation and new review. If a failed create has no trustworthy observation, preserve the `desired-intent-unavailable` resource/failed-command diagnostic, recover/reapply its declaration and investigate the provider outcome before removal. Discovery alone does not clear desired intent.

An unavailable literal secret cannot be reconstructed from history. Preserve its classification and the hashed-value error; obtain plaintext from an authorized source or keep a valid reference/generator expression. Never substitute a digest or write secrets to the local registry.
