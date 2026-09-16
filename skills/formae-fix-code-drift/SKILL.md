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
  unavailable, say it does not match the intended state without guessing who made it.

Name the actual property values and map keep to `absorb`, revert to `revert`.
Keep the user's requested change distinct: "Keep the outside change and add app=test" is
a combined infrastructure plan. Obtain one decision per actionable resource,
including any coupled properties. Source preparation, file paths and local
code edits stay internal. Acceptance is recorded in formae; never frame keeping
a change as adding it to a Pkl file. Mode `codebase` still synchronizes the
selected maintained project after central acceptance.

1. Call `apply_forma` with the original complete declaration, selected `context`, `mode: reconcile`, `simulate: true`, and no force or resolution. Inspect the structured rejection's `ObservationID`, stable `ResourceID`, and pinned observed origin. Present current actionable changes grouped by stack. Historical unavailable inputs or provenance remain unavailable.
2. Read the current `workflow.drift_preference` on the rejection (or `get_codebase_context`). Before the first decision, explain the guardrail in plain language: formae checks whether managed infrastructure changed elsewhere so it can protect the user from overwriting a change they may want to keep. Default/unset means ask: "A change was made outside formae: this bucket changed. Do you want to keep this change or revert it?" An instruction to add another label does not answer this question. First ask only about the current keep/revert decision. Only after the user explicitly chooses **keep** for a change, when `explicit: false` and preference storage is available, offer to ask each time or automatically keep future nonconflicting changes, including changes made by formae and deletions. Do not offer after revert alone. Explain that only conflicts and removals require a keep/revert decision; ordinary apply confirmation still applies. Save the explicit answer with `set_drift_preference`. Ask once during this operation, preferably immediately after keep; the successful resolution-preview response repeats the reminder if it was missed. Keeping this change does not consent to future automatic acceptance. An unanswered preference question does not block the current apply. A saved explicit choice means ask about each change without repeating the preference offer. If preference storage is unavailable, ask about the current change and explain that the preference could not be read; do not overwrite it. This preference belongs to the local user and resolved installation, across sessions/codebases.
   With `auto_absorb`, propose `absorb` for every actionable resource in the observation, including external changes, formae patches, mixed history and deletions. Only conflicts require a keep/revert decision. Keep origin wording factual; do not label a patch as an external edit. With the legacy `auto_absorb_external`, choose `absorb` automatically only for resources marked `ExternalChangesOnly: true` by the agent. The agent checks the complete interval since the desired baseline; the latest sync alone is insufficient. Under that legacy external-only preference, patches, mixed/unknown history and missing proof still require a keep/revert decision. Never silently broaden existing external-only consent. Keeping an external deletion removes that resource from desired state; show this explicitly in the final combined preview and message. Any conflicting requested edit or remaining dependency must be resolved before submission. Every actionable resource still needs one choice; no skip. Use the final resolution simulation's three-way merge to check conflicts. On `decision-edit-conflict`, ask the user; never discard the requested edit or pre-absorb source to manufacture success. Automatic acceptance does not bypass ordinary confirmation of cloud writes.
   On the first explicit **revert** for a stack, ask separately whether the user wants automatic reconciliation for that stack. Explain it as keeping the stack aligned with its intended state when something changes outside formae. Offer: install it for this stack, install it for all stacks, or decide separately for each stack. Check whether the stack already has the policy and whether this offer was already made before asking; do not repeat it. If the user chooses a policy, use `create_inline_policy` with `policy_type: auto_reconcile` and the normal policy workflow. Do not install it without explicit confirmation, and keep the current apply confirmation separate. The policy question applies to the first revert decision for each stack, not to a keep decision.
3. Simulate the same original complete declaration with `resolution: {ObservationID, Decisions: [{ResourceID, Action}, ...]}`. Show the final combined plan: acceptance records, provider writes for reverts, compatible user edits/additions/deletions, dependency propagation, and warnings. Absorb can coexist with required provider work; never label the whole plan write-free just because it contains acceptance.
4. Present a factual suggested `message` with the final plan and ask for confirmation. For example: "Keep external oob label and add app=demo. Suggested message: Accept external bucket label and add application label. Apply? You can edit or omit the message." Include automatic acceptance in both the preview and message. The user may accept, edit or clear the message in that same confirmation; clearing means `message: ""` and needs no separate approval. Do not include secrets.
5. After confirmation, submit the same original declaration and decisions with `simulate: false`, the returned `ReviewID`, and a caller-generated stable `IdempotencyKey`. Retain the exact input, final message and key until outcome is known. Retry an uncertain request with that identical submission and key; changing it causes an idempotency conflict. Never silently retry with force.
6. Use `get_command_status` with `wait: true`. Report the durable central outcome and command ID, distinguishing acceptance from provider changes. A terminal Failed command can contain desired contributions; report the actual failures.

`stale-review` requires a fresh initial observation, complete choices, final simulation and confirmation. If infrastructure source changes, restart that review. Message editing before admission does not require another review; message changes after admission cannot reuse the same key. For `decision-edit-conflict`, resolve the source conflict and re-review. Keep the original full declaration through choice review; do not edit it to pre-absorb the observation.

If a literal secret exists only as an opaque hash, retain the explicit hashed-value diagnostic. Obtain the required plaintext from an authorized source or preserve a valid reference/generator declaration. Never paste the hash as a value, invent plaintext, strip classification or save secrets into context metadata. For failed-create intent, inspect the failed command and actual observation. Retain it for retry, revise it, or explicitly omit it from the next complete reconcile according to the user's intended resolution. Omission records withdrawal even with zero cloud operations; it does not assert that the cloud resource is absent. An unresolved extraction reference must be repaired before Pkl evaluation, without silently dropping its owner or rebinding a same-name resource.

## Catch up the selected source

After the central command is terminal, mode `codebase` calls `get_command_desired_delta` for that command. This is **partial source-edit guidance**, including deleted declarations, not a complete reconcile file. Automatically update only the selected project through the harness's file editing tools, preserving abstractions, generators, references and unrelated edits. Validate the edited complete main forma with a soft reconcile simulation. Report source conflicts separately from central success; do not overwrite conflicting local work or say source is synchronized when it is not.

Mode `none` performs no maintained-source synchronization. Keep the disposable directory while a submission might need replay or inspection. Remove it after terminal outcome or after abandoning a preview that made no real submission. A future operation prepares a fresh complete desired extraction. Patch remains incident work. A later requested reconcile honors `auto_absorb` for nonconflicting patches; default prompt and legacy external-only consent still require a patch decision.


## No-codebase language

When the user is not maintaining an infrastructure codebase, translate this workflow into product language. Say “changes made outside formae” and “your current setup”; do not say drift, reconcile, absorb, force-sync, or drift decision in user-facing text. On the first detection, describe the safety guardrail positively: formae checks for outside changes so they are not overwritten unexpectedly, and the user can choose to keep or restore them. If keeping or restoring an outside change takes a separate operation before the requested update, say “I’ll record that change, then apply your update.” Afterward, report the result in terms of what is now kept or restored and what the user requested. Describe preferences as “your preference for handling changes made outside formae has been saved.” Internal terms remain acceptable in tool calls and private reasoning only.
