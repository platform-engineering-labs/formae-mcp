---
name: formae-drift
description: "Use when the user wants to check for infrastructure drift, see what changed out-of-band, or absorb/overwrite out-of-band changes into their IaC codebase"
---

# Drift Detection and Absorption

## MANDATORY RULE: Absorb = Edit + Simulate

When absorbing drift, you MUST run a reconcile simulation immediately after editing the PKL file — in the same turn, without asking, without pausing, without reporting success. An absorption is only complete when the simulation confirms no changes required. Telling the user "done" after an edit without simulating is WRONG.

## Workflow

### 1. Check for drift

Ask the user whether they want to check a specific stack or all stacks. Then call `list_drift` with the appropriate stack parameter (or omit it for all stacks).

### 2. Verify drift against IaC code

The drift endpoint reports modifications since the last reconcile. However, drift may have already been absorbed into the IaC code without a reconcile having been run since. To distinguish true drift from already-absorbed drift:

1. If `list_drift` returns modifications, ask the user for the path to their main forma file
2. Run `apply_forma` with `mode: reconcile`, `simulate: true`, `force: true` on that file
3. Cross-reference the results:
   - Resources in the drift list that also appear as **updates** in the simulation → **true drift** (code doesn't match cloud)
   - Resources in the drift list but the simulation shows **no changes** for them → **already absorbed** (code was updated but no reconcile has been run yet)

Only present true drift to the user. For already-absorbed drift, mention that those resources were previously absorbed and will clear on the next reconcile.

### 3. Present results

**If no true drift:** Report that all stacks are clean — any reported drift has already been absorbed and will clear on the next reconcile.

**If true drift is detected:** Present the modifications grouped by stack, showing:
- Resource type (e.g., `AWS::S3::Bucket`)
- Resource label
- Operation (`update` = properties changed, `delete` = resource was removed outside formae)

### 4. Ask what to do

For each drifted resource (or group of resources), ask the user what action to take:

- **Absorb**: Incorporate the out-of-band change into the IaC codebase so the code matches what's actually deployed
- **Overwrite**: Force-reconcile to push the desired IaC state back to the cloud, reverting the out-of-band change
- **Skip**: Leave it for now

### 5. Absorb workflow

When the user chooses absorb, you MUST execute all of (a) through (e) in a single uninterrupted sequence:

(a) Call `extract_resources` with a query matching the drifted resource

(b) Read the existing IaC codebase to understand how the resource is currently defined

(c) Edit the PKL source to match the extracted (actual) state — only change what drifted

(d) In the SAME turn, without pausing: call `apply_forma` with `mode: reconcile`, `simulate: true`, `force: true` on the **main forma file**. Then call `get_command_status` to check the result.

(e) Evaluate the simulation:
   - **No changes required** → report success to the user with the simulation as evidence
   - **Changes remain** → fix the PKL and loop back to (d) without asking
   - **Unexpected side effects** → stop and ask the user

### 6. Overwrite workflow

For resources the user wants to overwrite:

1. Run `apply_forma` with `mode: reconcile`, `simulate: true`, `force: true` on the main forma file
2. Present the simulation showing what will be pushed back to the cloud
3. **Ask for explicit confirmation** before proceeding
4. Run `apply_forma` with `mode: reconcile`, `simulate: false`, `force: true`
5. Monitor with `get_command_status` and report the result

### 7. Post-workflow

After handling all drifted resources, re-run the verification from step 2 to confirm all remaining drift has been absorbed. Note that `list_drift` alone may still report drift for absorbed resources — this is expected and will clear on the next reconcile.

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER report an absorption as complete without a simulation proving it is a no-op
- NEVER overwrite without user confirmation
- When absorbing, only modify the specific properties that drifted — do not restructure or rewrite unrelated code
- Resource labels must match exactly — never rename them
- If the user has multiple drifted resources, handle them one at a time or in logical groups as the user prefers
