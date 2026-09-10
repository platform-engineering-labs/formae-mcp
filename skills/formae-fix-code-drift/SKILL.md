---
name: formae-fix-code-drift
description: "Use when the user wants to check infrastructure drift or explicitly absorb or revert out-of-band changes, with or without a maintained IaC codebase"
---

# Resolve Infrastructure Drift

Absorb records accepted desired intent centrally. Revert restores the previous eligible desired declaration. A preview or source edit alone does not finish acceptance.

## Select context

Pass the selected `profile` on every agent call; never switch the global active profile to prepare a session. Call `get_codebase_context` with the actual harness `working_directory`, or reuse the explicit selection for this operation. Follow `formae-author` for selection and disposable source preparation. A hosted user with mode `none` needs no maintained project. Known candidates require selection once; missing directories and corrupt registration are actionable errors.

Check the connected installation's `shared-drift-resolution` and `desired-stack-extraction` capabilities. A local binary version does not establish server support. An old agent requires a complete existing codebase and cannot perform this recorded resolution workflow; explain that limitation without sending unsupported controls.

For mode `codebase`, read the selected main forma and preserve its original declaration. For mode `none`, create an empty disposable directory and call `prepare_authoring` for the complete affected stacks. Keep its returned `context`, PklProject, dependencies and full Pkl files. Desired extraction supplies accepted intent, not a claim that current drift is settled. Never prepare a full reconcile from filtered inventory or a command delta.

## Observe, decide, review, submit

1. Call `apply_forma` with the original complete declaration, selected `context`, `mode: reconcile`, `simulate: true`, and no force or resolution. Inspect the structured rejection's `ObservationID`, stable `ResourceID`, and pinned observed origin. Present current actionable changes grouped by stack. Historical unavailable inputs or provenance remain unavailable.
2. Obtain exactly one explicit `absorb` or `revert` choice for every actionable `ResourceID` in the observation. Coupled properties are one resource choice. There is no skip within that stack's resolution; the user can abandon the operation without submitting.
3. Simulate the same original complete declaration with `resolution: {ObservationID, Decisions: [{ResourceID, Action}, ...]}`. Show the final combined plan: acceptance records, provider writes for reverts, compatible user edits/additions/deletions, dependency propagation, and warnings. Absorb can coexist with required provider work; never label the whole plan write-free just because it contains acceptance.
4. Present a factual suggested `message` with the final plan and ask for confirmation. The user may accept, edit or clear the message in that same confirmation; clearing means `message: ""` and needs no separate approval. Do not include secrets.
5. After confirmation, submit the same original declaration and decisions with `simulate: false`, the returned `ReviewID`, and a caller-generated stable `IdempotencyKey`. Retain the exact input, final message and key until outcome is known. Retry an uncertain request with that identical submission and key; changing it causes an idempotency conflict. Never silently retry with force.
6. Use `get_command_status` with `wait: true`. Report the durable central outcome and command ID, distinguishing acceptance from provider changes. A terminal Failed command can contain desired contributions; report the actual failures.

`stale-review` requires a fresh initial observation, complete choices, final simulation and confirmation. If infrastructure source changes, restart that review. Message editing before admission does not require another review; message changes after admission cannot reuse the same key. For `decision-edit-conflict`, resolve the source conflict and re-review. Keep the original full declaration through choice review; do not edit it to pre-absorb the observation.

If a literal secret exists only as an opaque hash, retain the explicit hashed-value diagnostic. Obtain the required plaintext from an authorized source or preserve a valid reference/generator declaration. Never paste the hash as a value, invent plaintext, strip classification or save secrets into context metadata. For `desired-intent-unavailable` naming a failed create, investigate the resource and failed command, recover/reapply its declaration, then consider removal. Discovery alone does not clear that intent; force does not bypass recovery.

## Catch up the selected source

After the central command is terminal, mode `codebase` calls `get_command_desired_delta` for that command. This is **partial source-edit guidance**, including deleted declarations, not a complete reconcile file. Automatically update only the selected project through the harness's file editing tools, preserving abstractions, generators, references and unrelated edits. Validate the edited complete main forma with a soft reconcile simulation. Report source conflicts separately from central success; do not overwrite conflicting local work or say source is synchronized when it is not.

Mode `none` performs no maintained-source synchronization. Keep the disposable directory while a submission might need replay or inspection. Remove it after terminal outcome or after abandoning a preview that made no real submission. A future operation prepares a fresh complete desired extraction. Patch remains incident work and is never automatically absorbed.
