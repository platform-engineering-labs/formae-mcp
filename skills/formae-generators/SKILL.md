---
name: formae-generators
description: "Use when the user wants a credential generated and rotated by formae rather than written into the forma — e.g. 'rotate the database password every 90 days', 'stop hardcoding this secret', 'generate the API key', 'what rotates in this stack?', 'when did that credential last rotate?', 'why has this not rotated?'. Covers declaring a generator, binding a resource property to it, and inspecting rotation state."
---

# Generators — credentials formae draws and rotates

A **generator** draws a credential and hands it to the resource properties bound to it. If it declares a cadence it also rotates that credential on schedule, unattended.

The value itself never leaves the agent's memory: it is written to the destination and hashed at rest. Nothing — not the plan, not the inventory, not the logs — shows it. What you can see is the generation's identity and which resources it feeds.

## What a generator replaces

The pattern it exists to remove is a credential minted at eval time and then pinned so it stops moving:

```pkl
local dbPassword = random.password(32, false)
...
secretString = formae.value(dbPassword).opaque.setOnce
```

That mints a new value on **every evaluation**, so `setOnce` is doing the work of stopping every apply from rotating the credential. Replacing the seed with a generator removes the need for `setOnce` and makes rotation something you asked for rather than something you are preventing.

## Inspecting what rotates

Use `list_generators`. It returns each generator's label, stack, type, generation spec, cadence, the instant of its last committed rotation, and the resources bound to it.

| User asks... | Answer from |
|---|---|
| "What rotates in this stack?" | the generator list, filtered by stack |
| "When did that credential last rotate?" | `LastRotatedAt` |
| "When does it next rotate?" | `LastRotatedAt` plus `EverySeconds` |
| "What does this generator feed?" | `Destinations` |
| "Is this credential generated or declared?" | whether the resource appears as a destination |

`LastRotatedAt` is derived from command history rather than stored on the resource, so it never appears in a diff and never causes a spurious update.

An agent that predates generators reports none. That is the truthful answer, not an error.

## Authoring — declare, then bind

A generator is a top-level forma entry, like a stack or a target. A property binds to one of its named outputs.

```pkl
local dbPasswordGen: formae.PasswordGenerator = new {
  label = "db-password-gen"
  stack = appStack.res
  length = 32
  symbols = false
  rotation = new formae.RotationSpec {
    every = 90.d
  }
}

local dbPasswordSecret: secret.Secret = new {
  label = "db-password"
  name = "app/db-password"
  secretString = dbPasswordGen.gen.value
  stack = appStack.res
  target = awsTarget.res
}
```

Then collect both in `forma { ... }`.

**Omit `rotation` to draw once and never rotate.** That is the right shape for a credential that should be generated rather than written down but has no reason to turn over on a schedule, and it is the direct replacement for the `random.password` + `setOnce` pattern above.

## Constraints that will bite

**Every destination must be in the same apply.** A generator that draws must reach all of its destinations in one command, or the apply is refused naming the ones it cannot reach. This is not a limitation to work around: the drawn value exists only for the duration of that command, so a destination left out can never be caught up without drawing again and putting its siblings behind. If a generator feeds resources on two stacks, apply both together.

**The cadence floor is one minute.** Anything shorter is refused at evaluation. A credential that must turn over faster than that wants short-lived credentials issued per use, not a scheduler revisiting a long-lived one.

**A field the schema does not mark opaque will store the value in cleartext.** Bind generators to fields that are declared as secret-bearing. If a plugin's field takes a generator output, it is opaque by construction.

**Rotation refuses on a drifted stack.** Rotation is an ordinary update, so it will not overwrite an out-of-band change. A stack carrying an auto-reconcile policy is the standing opt-in to overwrite drift, and there rotation proceeds.

**Destroying a stack whose generator is referenced elsewhere** aborts, naming the referencing resources, unless `--on-dependents=cascade` is given.

## Know this before promising rotation to a user

A resource that takes its value from a generator-fed secret **by reference** — the consuming side of `secret.res.secretValue` — does not currently follow a rotation. The secret is updated; the referencing resource is not, until the next ordinary apply.

The practical consequence, and it is worth saying out loud rather than discovering: rotating a database password updates the stored secret while the database still expects the old one, so anything that reads the secret to authenticate will be rejected until someone re-applies.

So when a user asks for rotation on a credential that something else consumes by reference, say what will and will not follow. A generator feeding a single destination that is itself the authority is unaffected.

## Simulate first

`apply_forma` with `simulate=true` shows the generator operation, whether a draw is impending, and which destinations it moves — with no material. An apply that plans no rotation says so, which makes "nothing will rotate" an assertion rather than an absence.
