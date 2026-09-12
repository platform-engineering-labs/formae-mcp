---
name: formae-fix-code-drift
description: "Use when the user wants to check infrastructure drift or explicitly absorb or revert out-of-band changes, with or without a maintained IaC codebase"
---

# Resolve Infrastructure Drift

Absorb records accepted desired intent centrally. Revert restores the previous eligible desired declaration. A preview or source edit alone does not finish acceptance.

## Select context

Pass the selected `profile` on every agent call; never switch the global active profile to prepare a session. Call `get_codebase_context` with the actual harness `working_directory`, or reuse the explicit selection for this operation. Follow `formae-author` for selection and disposable source preparation. A hosted user with mode `none` needs no maintained project. Known candidates require selection once; missing directories and corrupt registration are actionable errors.

Check the connected installation's `shared-drift-resolution` and `desired-stack-extraction` capabilities. A local binary version does not establish server support. An old agent requires a complete existing codebase and cannot perform this recorded resolution workflow; explain that limitation without sending unsupported controls.

For mode `codebase`, read the selected main forma and preserve its original declaration. For mode `none`, create a fresh `~/.formae-ai/scratch/<operation-id>/` directory and call `prepare_authoring` for the complete affected stacks. Keep its returned `context`, PklProject, dependencies and full Pkl files. Desired extraction supplies accepted intent, not a claim that current drift is settled. Never prepare a full reconcile from filtered inventory or a command delta.

## Observe, decide, review, submit

In both codebase modes, explain the infrastructure decision, using the pinned observed
origin rather than treating every difference as external:

Read each resource's `ObservedCommand`, `ObservedMode`, `ObservedSource` and
`ObservedCommandID` from the rejection's `ModifiedStacks[].ModifiedResources`
(ModifiedStacks is keyed by stack). `ObservedCommand: "sync"` or
`ObservedSource: "synchronizer"` identifies a sync observation;
`ObservedCommand: "apply"` with `ObservedMode: "patch"` identifies
a formae patch. `ObservedSource` describes the command source, not an actor's
authenticated identity. These fields identify the pinned observation, not
necessarily every earlier contribution to a cumulative diff. Do not attribute
all properties to its latest command when multiple changes may have accumulated;
explain mixed or unavailable history accurately.

- **Proven external changes:** "The bucket's oob label was added outside formae. Keep it or revert it?" A latest sync alone does not prove every cumulative change was external.
- **Patch:** "An earlier formae patch changed the bucket's labels. Keep that
  change as the stack's desired state or revert it?"
- **Mixed/unknown:** describe the known contributions together; when origin is
  unavailable, say it differs from the last reconcile without guessing who made it.

Name the actual property values and map keep to `absorb`, revert to `revert`.
Keep the user's requested change distinct: "Keep oob=drift and add app=test" is
a combined infrastructure plan. Obtain one decision per actionable resource,
including any coupled properties. Source preparation, file paths and local
code edits stay internal. Acceptance is recorded in formae; never frame keeping
a change as adding it to a Pkl file. Mode `codebase` still synchronizes the
selected maintained project after central acceptance.

1. Call `apply_forma` with the original complete declaration, selected `context`, `mode: reconcile`, `simulate: true`, and no force or resolution. Inspect the structured rejection's `ObservationID`, stable `ResourceID`, and pinned observed origin. Present current actionable changes grouped by stack. Historical unavailable inputs or provenance remain unavailable.
2. Read the current `workflow.drift_preference` on the rejection (or `get_codebase_context`). Default/unset means ask: "A change was made outside formae: oob=drift was added to this bucket. Keep it or revert it?" An instruction to add another label does not answer this question. First ask only about the current keep/revert decision. Only after the user explicitly chooses **keep** for a change whose original observation has `ExternalChangesOnly: true`, when `explicit: false` and preference storage is available, offer to ask each time or automatically keep future nonconflicting external changes, including external deletions. Do not offer after revert alone, a patch, or mixed/unknown history. Ask once during this operation, preferably immediately after keep; the successful resolution-preview response repeats the reminder if it was missed. Keeping this change does not consent to future automatic acceptance. An unanswered preference question does not block the current apply. A saved explicit `prompt` choice means ask about each change without repeating the preference offer. If preference storage is unavailable, ask about the current change and explain that the preference could not be read; do not overwrite it. Call `set_drift_preference` only with the user's explicit choice; silence keeps the default. This preference belongs to the local user and resolved installation, across sessions/codebases.
   With `auto_absorb_external`, choose `absorb` automatically only for resources marked `ExternalChangesOnly: true` by the agent. The agent checks the complete interval since the desired baseline; the latest sync alone is insufficient. Patches, mixed/unknown history and missing proof still require a keep/revert decision. Keeping an external deletion removes that resource from desired state; show this explicitly in the final combined preview and message. Any conflicting requested edit or remaining dependency must be resolved before submission. Every actionable resource still needs one choice; no skip. Use the final resolution simulation's three-way merge to check conflicts. On `decision-edit-conflict`, ask the user; never discard the requested edit or pre-absorb source to manufacture success. Automatic acceptance does not bypass ordinary confirmation of cloud writes.
3. Simulate the same original complete declaration with `resolution: {ObservationID, Decisions: [{ResourceID, Action}, ...]}`. Show the final combined plan: acceptance records, provider writes for reverts, compatible user edits/additions/deletions, dependency propagation, and warnings. Absorb can coexist with required provider work; never label the whole plan write-free just because it contains acceptance.
4. Present a factual suggested `message` with the final plan and ask for confirmation. For example: "Keep external oob label and add app=demo. Suggested message: Accept external bucket label and add application label. Apply? You can edit or omit the message." Include automatic acceptance in both the preview and message. The user may accept, edit or clear the message in that same confirmation; clearing means `message: ""` and needs no separate approval. Do not include secrets.
5. After confirmation, submit the same original declaration and decisions with `simulate: false`, the returned `ReviewID`, and a caller-generated stable `IdempotencyKey`. Retain the exact input, final message and key until outcome is known. Retry an uncertain request with that identical submission and key; changing it causes an idempotency conflict. Never silently retry with force.
6. Use `get_command_status` with `wait: true`. Report the durable central outcome and command ID, distinguishing acceptance from provider changes. A terminal Failed command can contain desired contributions; report the actual failures.

`stale-review` requires a fresh initial observation, complete choices, final simulation and confirmation. If infrastructure source changes, restart that review. Message editing before admission does not require another review; message changes after admission cannot reuse the same key. For `decision-edit-conflict`, resolve the source conflict and re-review. Keep the original full declaration through choice review; do not edit it to pre-absorb the observation.

If a literal secret exists only as an opaque hash, retain the explicit hashed-value diagnostic. Obtain the required plaintext from an authorized source or preserve a valid reference/generator declaration. Never paste the hash as a value, invent plaintext, strip classification or save secrets into context metadata. For failed-create intent, inspect the failed command and actual observation. Retain it for retry, revise it, or explicitly omit it from the next complete reconcile according to the user's intended resolution. Omission records withdrawal even with zero cloud operations; it does not assert that the cloud resource is absent. An unresolved extraction reference must be repaired before Pkl evaluation, without silently dropping its owner or rebinding a same-name resource.

## Catch up the selected source

After the central command is terminal, mode `codebase` calls `get_command_desired_delta` for that command. This is **partial source-edit guidance**, including deleted declarations, not a complete reconcile file. Automatically update only the selected project through the harness's file editing tools, preserving abstractions, generators, references and unrelated edits. Validate the edited complete main forma with a soft reconcile simulation. Report source conflicts separately from central success; do not overwrite conflicting local work or say source is synchronized when it is not.

Mode `none` performs no maintained-source synchronization. Keep the disposable directory while a submission might need replay or inspection. Remove it after terminal outcome or after abandoning a preview that made no real submission. A future operation prepares a fresh complete desired extraction. Patch remains incident work and is never automatically absorbed.
