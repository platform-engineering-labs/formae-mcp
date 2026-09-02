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

**The cadence floor is one minute, and the provider's ceiling is lower than you think.** Anything shorter than a minute is refused at evaluation, and a credential that must turn over faster wants short-lived credentials issued per use, not a scheduler revisiting a long-lived one. But the floor is not a recommendation: AWS Secrets Manager retains every version from the last 24 hours against a non-adjustable quota of 100 versions per secret and advises against sustained writes more often than once per 10 minutes, so a one-minute cadence exhausts the quota in under two hours and every rotation after that fails. Treat minutes-scale cadences as drill settings, never as a steady state; a sustained cadence should be 15 minutes at the absolute fastest.

**A field the schema does not mark opaque will store the value in cleartext.** Bind generators to fields that are declared as secret-bearing. If a plugin's field takes a generator output, it is opaque by construction.

**Rotation refuses on drift, and it sees the consumers too.** Rotation is an ordinary update, so it will not overwrite an out-of-band change — and because it plans the resources consuming the rotated credential as well as its destinations, a drifted *consumer* also stops it. A stack carrying an auto-reconcile policy is the standing opt-in to overwrite drift, and there rotation proceeds.

**Destroying a stack whose generator is referenced elsewhere** aborts, naming the referencing resources, unless `--on-dependents=cascade` is given.

## The window, and the contract to state for consumers

A rotation moves everything downstream of the credential in one command: the generator's destinations (the secret) and, transitively, the resources that consume them by reference (the database role whose password is the secret's value, and the database owned by that role). This needs formae 0.89.0 or newer; on an older agent only the destinations move, and a resource consuming them by reference stays on the old value until the next ordinary apply — check with `check_health` and say so before promising rotation.

What no rotation removes is the **window** between the two writes. No transaction spans the secret store and the system that accepts the credential, so for a moment the secret advertises a password the engine does not yet accept — and for up to a consumer's own cache lifetime afterwards, a cached reader keeps presenting one the engine no longer accepts. With a single credential no write ordering avoids this; only overlapping validity (two alternating users) removes it, and formae does not orchestrate that today.

So when a user asks for rotation on a credential that something reads to authenticate, state the consumer contract out loud:

- **The consumer must re-read the credential** at least once per rotation period — per connection, or on a bounded cache TTL. A consumer that reads it once at startup breaks on the first rotation and stays broken until restarted.
- **The consumer must tolerate transient authentication failures around a rotation**, for up to its own cache lifetime, by retrying or reconnecting. The failures end on their own when its cache expires.

The formae agent is itself a worked example of the contract: pointing `datastore.postgres.passwordSecretArn` at the secret makes it resolve its own database password per new connection, and it rides out a rotation of that credential without a restart.

## Simulate first

`apply_forma` with `simulate=true` shows the generator operation, whether a draw is impending, and which destinations it moves — with no material. An apply that plans no rotation says so, which makes "nothing will rotate" an assertion rather than an absence.
