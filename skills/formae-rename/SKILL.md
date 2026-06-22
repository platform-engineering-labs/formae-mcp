---
name: formae-rename
description: "Use when the user wants to rename a managed resource, relabel a resource, change a resource's label, or give a discovery-named resource a readable name — a rename on its own never destroys or recreates the cloud object"
---

# Rename a Resource (`alias`)

Change a resource's `label` without destroying and recreating the cloud object. A rename **on its own** is non-destructive — no cloud operation at all. If you change properties in the same edit, the property change (not the rename) decides the outcome: a mutable change is an in-place update, an immutable (`createOnly`) change forces a destroy-and-recreate `replace`. See "Reading the simulation." The rename happens inline with `apply_forma` — there is no separate rename command.

## How rename works

A `label` is a formae-side identifier; it is never sent to the cloud provider. To rename, set `alias` to the resource's **previous** label and `label` to the **new** one:

```pkl
new vpc.VPC {
  label = "production-vpc"   // the NEW label
  alias = "vpc-008eef..."    // the PREVIOUS label
  cidrBlock = "172.31.0.0/16"
}
```

On apply, the agent matches the existing managed row by `alias` and renames it in place. KSUID and the cloud NativeID are preserved — no destroy/recreate. Cross-resource `.res` references that resolve through the renamed resource's KSUID keep working.

A rename can ride along with a property change in the same apply (one update carries both).

## Workflow

1. **Locate the resource in the codebase.** Find the `label` the user wants to change in their forma/PKL files. Confirm the current label and the desired new label with the user.
2. **Edit the resource.** Set `alias` to the current label, change `label` to the new name. Leave properties untouched unless the user also asked for a change.
3. **Simulate**: call `apply_forma` with `mode: reconcile` (or `patch`), `simulate: true`.
4. **Check the simulation** against the cases in "Reading the simulation" below. A pure rename is a single `update` with a `change label from "<old>" to "<new>"` line and nothing else. **If the simulation shows a `replace`, stop** — an immutable field changed alongside the rename, and applying will destroy and recreate the cloud object.
5. **Ask for explicit confirmation**, then apply with `simulate: false`.
6. **Monitor** with `get_command_status`:
   - Wait 5 seconds between polls (`sleep 5`). Do NOT poll in a tight loop.
   - Only report state transitions. Summarize what changed rather than dumping JSON.
7. **Report** the result. Mention the `alias` can now stay or be removed (re-applies match by the new label first, so the alias is dead-but-harmless).

## Reading the simulation

The rename itself never destroys the cloud object. What the simulation shows depends on whether you also changed properties in the same edit:

- **Pure rename** — a single `update` whose only change is `change label from "<old>" to "<new>"` (no property patch, no create, no delete). Safe to apply after confirmation.
- **Rename + mutable property change** — an `update` carrying the `change label` line plus a property patch. Still in-place; the cloud object is modified, not recreated. Safe to apply once the user confirms the property change.
- **Rename + immutable (`createOnly`) property change → `replace`** — **DESTRUCTIVE.** The `replace` is driven by the immutable field, not the rename: formae will destroy and recreate the cloud object (losing anything the provider doesn't preserve across recreation). Do NOT treat this as a normal rename outcome. Stop, tell the user plainly that applying will destroy and recreate the resource because of the immutable-field change, and get explicit confirmation — or split the work: do the rename on its own first (non-destructive), then make the property change separately.
- **NOT acceptable — create + delete**: the `alias` doesn't match any existing managed row. Check the alias is the exact current label, same stack, same type. Fix and re-simulate.
- **Apply rejected** with an alias error: see the three pre-flight rejections under "Constraints".

## Constraints

The agent validates `alias` before touching the cloud and rejects:

- **Two resources claim the same row** — one references it by current label, another via `alias`. Remove the duplicate declaration.
- **Dead alias** — `alias` names a label that matches no existing managed (same stack) or unmanaged resource of the same type. Fix or remove it.
- **`alias` equals `label`** — the alias must reference a *different* prior label.

## Renaming while importing

If the resource is still unmanaged (discovered, not yet in a forma), don't use this skill — use `/formae-import` and set `alias` to the discovery-assigned label during the import. The resource is adopted and renamed in one apply.

## Scope

- Renames a resource's `label` only. Moving a resource between stacks or targets is a separate operation.
- Stack rename and target rename are not yet supported.

## Important

- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`. Forma files use formae-specific extensions that only the formae CLI can resolve, and `--output-consumer machine` ensures parseable output instead of human-formatted text.
- NEVER skip the simulation step
- NEVER apply without user confirmation
- Only set `alias` when the user actually wants a rename — a stray `alias` that matches nothing is rejected at apply time
