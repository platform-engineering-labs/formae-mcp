---
name: formae-apply
description: "Use when the user wants to deploy infrastructure, apply a forma file, reconcile a stack, update a stack, or make planned infrastructure changes"
---

# Apply Infrastructure

Use `apply_forma` in reconcile mode for planned changes. Each explicitly declared stack is a complete resource boundary: omitted resources are deleted, including when a stack contains only generators. Patch is the scoped incident path.

Ordinary updates use a complete declaration of the affected stack with mode=reconcile and force=false (or omitted), even for one resource or one label. In no-codebase mode, prepare the complete desired stack, modify it, and carry the returned context on both simulation and real submission. In maintained-codebase mode, use the selected complete source and binding context. Small scope, 'quick' wording, a patch-named file, or detected drift does not authorize patch. Use patch only when the user explicitly requests patch mode or their stated incident/hotfix intent calls for a temporary intervention that defers reconciliation. Never switch to patch or force to bypass a drift rejection; follow the explicit keep/revert workflow. If only partial source is available, retrieve the complete desired stack rather than changing the apply mode.

## Source and installation

For infrastructure apply/authoring, resolve source context before any IaC file search or read: reuse the explicit selection already made for this installation and workspace, or call get_codebase_context with the actual harness working_directory and selected profile. This tool consults the registry; do not replace it with rg/find/glob scans. Mode none is a complete selection, not a missing project to recover. Keep that selection for subsequent operations; do not search again merely because the user asks for another resource change. Resolve again when the installation/workspace changes, the user changes their codebase choice, or a context-validation error requires it. In none mode, locate the affected stack through formae inventory if needed, then prepare_authoring for its complete desired declaration in a fresh disposable directory. Ignore old/unregistered Pkl folders, remembered paths, previous temporary files and Git status. A value in an unselected local file does not establish pending desired intent; inspect formae desired extraction and recorded commands. In codebase mode, read/search only the selected registered root; a missing file or source conflict is an error to resolve there, not permission to search elsewhere. Never register a discovered folder without explicit user opt-in. If selection is required, present registered candidates; do not search for additional candidates. A source-only question about a user-supplied local file can read that exact file without resolving or adopting an infrastructure context; do not broaden that into a project search.

Pass the chosen `profile` on every agent call; never change the global active profile for a session. Call `get_codebase_context` with the actual harness `working_directory` or reuse the explicit context already selected. Resolve known candidates once and surface missing/corrupt projects.

- `codebase`: carry `context: {mode: "codebase", binding_id: ...}` and use the complete main forma inside that selected project. Every evaluated stack must fit its registered scope. Preserve abstractions and unrelated edits.
- `none`: follow `formae-author` to call `prepare_authoring` in a fresh `~/.formae-ai/scratch/<operation-id>/` directory. Carry its returned `context` and complete `file_path`; keep PklProject and resolved dependencies. Never use an actual-inventory fragment or partial command delta as full reconcile input. A maintained project is opt-in.
- Older agents without the connected `desired-stack-extraction` and `shared-drift-resolution` capabilities require an existing complete codebase. Do not send new controls or imply local binary version proves server support. Legacy file-based apply remains available.

Pkl remains the code interface in both modes. Use `formae eval --output-schema json --output-consumer machine` for local evaluation. If retaining an evaluated JSON file for exact retry, keep it inside the same selected workspace along with the Pkl source and dependencies. Never edit the evaluated submission after review.

## Workflow

For mode `none`, use the MCP conversation contract throughout: progress and
confirmation describe targets, stacks, resources and before/after values;
results describe actual outcomes and command IDs. Temporary source, filenames,
diffs, extraction queries and cleanup stay internal. Ask "Apply these changes?"
about the infrastructure plan, not approval to edit or synchronize files.
For `codebase`, retain relevant source reporting and selected-project catch-up.
Use the launcher-selected executable reported in MCP initialization for local
`formae eval`; it normally lives at `~/.formae-ai/opt/bin/formae` and need not be
on the harness PATH. Tool calls remain preferred where available.

1. Prepare the complete declaration, preserving every existing resource, target, policy, reference and generator in each affected stack unless its change/removal is intended.
2. Call `apply_forma` with the selected `context`, `mode: reconcile`, `simulate: true`, and no force.
3. If rejected for actionable drift, follow `formae-fix-code-drift`: use the initial `ObservationID`, all explicit absorb/revert choices, final composed simulation `ReviewID`, then confirmed real submission with a stable `IdempotencyKey`. Do not pre-edit the source to absorb drift. A stale review requires a fresh review and confirmation.
4. Show the final combined plan, including acceptance, provider work, ordinary changes, deletes, dependencies and warnings. Suggest a factual optional `message` in this same confirmation. The user can edit it or clear with `message: ""`; no extra confirmation round is needed solely for the message.
5. After confirmation, submit the exact reviewed input with `simulate: false`. Carry the final resolution controls when applicable. Retain exact input, message and idempotency key while outcome is uncertain; an idempotent replay uses all of them unchanged.
6. Call `get_command_status` with `wait: true`, and continue waiting if its budget ends before a terminal state. Report actual outcomes, errors, command ID and recorded acceptance separately from provider work. A no-change preview does not record central acceptance.
7. After a terminal resolution, use `get_command_desired_delta` to automatically catch up only the selected maintained codebase via harness source edits. Its `Partial` result is a snippet, never a full reconcile file. Preserve abstractions and unrelated edits, apply recorded deletion guidance, and simulate the complete main forma afterward. Report source conflicts separately from central command success. Never catch up from later unconstrained inventory.
8. For mode `none`, retain the disposable directory through outcome/retry inspection, then remove it. Also clean up abandoned previews after confirming no real request was sent. Never clean up an uncertain submission before retaining what is needed for its exact replay.

A Failed command remains in history with its recorded outcome. Diagnose outstanding failures before preparing a corrective reconcile. The user may retain intent for retry, revise the declaration, or abandon it. Explicitly omitting a failed-create declaration from a complete reconcile withdraws that intent even when there are zero cloud operations; the recorded reconcile prevents it returning on the next desired extraction. Withdrawal does not prove cloud absence or undo partial effects: preserve uncertainty and investigate the provider outcome separately. Do not remove a declaration merely because its creation failed.

An unavailable literal secret cannot be reconstructed from history. Preserve its classification and the hashed-value error; obtain plaintext from an authorized source or keep a valid reference/generator expression. Never substitute a digest or write secrets to the local registry.
