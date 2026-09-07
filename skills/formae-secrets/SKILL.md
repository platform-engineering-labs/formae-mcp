---
name: formae-secrets
description: "Use when the user wants formae to manage a credential: generate a password, rotate a secret, wire a database password or key pair, reference a secret from another resource, or ask what rotates and when something last rotated. For example 'generate the db password', 'rotate this every 90 days', 'is this secret managed by formae?', 'when did the identity key last rotate?', 'why has this not rotated?'"
---

# Secrets and generators

Use this skill to reference a secret, to author generator-backed credentials,
and to inspect rotation state. Requires formae >= 0.89.0 (the `list_generators`
tool refuses on older versions; treat that refusal as "this formae predates
generators").

## The two rules

1. **Reference, don't store.** A secret is an ordinary managed resource; other
   resources consume it by reference (`secret.res.secretValue`, `.at("key")`,
   `.json("path")`). The value is read live from the provider at each plugin
   call and is hashed at rest; never copy a secret's value into another
   property or into the forma text.
2. **Let formae draw the value.** When the user needs a NEW credential, declare
   a generator instead of inventing a value or using eval-time randomness. The
   agent draws it cryptographically, writes it to every bound destination in
   one command, and keeps only a digest.

## Authoring a generator-backed credential

A generator is a top-level forma entry, like a stack or a target: declare it,
then mention it inside `forma { ... }` alongside the resources that bind to it.

```pkl
local dbPasswordGen: formae.PasswordGenerator = new {
  label = "db-password-gen"
  stack = stackRef
  length = 32
  symbols = false
  rotation = new formae.RotationSpec { every = 90.d }
}

local dbPassword: secret.Secret = new {
  label = "db-password"
  name = "/app/db-password"
  secretString = dbPasswordGen.gen.value
  stack = stackRef
  target = targetRef
}
```

Without a `rotation` block the generator never rotates on a schedule, but do
not promise the value is fixed for life: an apply can still redraw it, when a
new destination binds to the generator, or when the generator's spec changes so
the held generation no longer satisfies it. The consumer contract below applies
to those redraws too. Key pairs: `formae.KeyPairGenerator` draws an RSA pair as
two named outputs of one draw, so bind `gen.privateKey` (PKCS#8 PEM) and
`gen.publicKey` (PKIX PEM) to their own destinations and the two always hold
halves of the same pair.

**Do NOT author the legacy shape** (eval-time `random.password(...)` seeded
into a `.opaque.setOnce` secret). It cannot rotate, and `setOnce` is a one-way
door: a resource created on that shape can never adopt a generator without
being destroyed and recreated.

## Choosing a cadence

- Steady state: days. 30 or 90 days (`30.d`, `90.d`) are the common choices.
- **The schema floor is fifteen minutes, enforced at evaluation.**
  `RotationSpec.every` refuses anything shorter, so a cadence of seconds or a
  few minutes is an eval-time error rather than a fast rotation. The floor is
  set by what the common secret store survives: AWS Secrets Manager retains
  every version from the last 24 hours against a non-adjustable quota of 100
  versions per secret and advises against sustained writes more often than once
  every 10 minutes.
- A credential that must turn over faster than the floor wants short-lived
  per-use credentials, not a scheduler revisiting a long-lived one.

State the chosen cadence to the user before applying, and simulate first.

## Constraints that will bite

**Every destination must be in the same apply.** A generator that draws must
reach all of its destinations in one command, or the apply is refused naming
the ones it cannot reach. This is not a limitation to work around: the drawn
value exists only for the duration of that command, so a destination left out
can never be caught up without drawing again and putting its siblings behind.
If a generator feeds resources on two stacks, apply both together.

**A field the schema does not mark opaque will store the value in cleartext.**
Bind generators to fields that are declared as secret-bearing. If a plugin's
field takes a generator output, it is opaque by construction.

**Rotation refuses on drift, and it sees the consumers too.** Rotation is an
ordinary update, so it will not overwrite an out-of-band change, and because it
plans the resources consuming the rotated credential as well as its
destinations, a drifted *consumer* also stops it. A stack carrying an
auto-reconcile policy is the standing opt-in to overwrite drift, and there
rotation proceeds.

**Destroying a stack whose generator is referenced elsewhere** aborts, naming
the referencing resources, unless `--on-dependents=cascade` is given.

## What a rotation does, and the contract to state for consumers

A rotation moves everything downstream of the credential in one command: the
generator's destinations (the secret) and, transitively, the resources that
consume them by reference (the database role whose password is the secret's
value, and the database owned by that role). This needs formae 0.89.0 or newer;
on an older agent only the destinations move, and a resource consuming them by
reference stays on the old value until the next ordinary apply. Check with
`check_health` and say so before promising rotation.

What no rotation removes is the **window** between the two writes. No
transaction spans the secret store and the system that accepts the credential,
so for a moment the secret advertises a password the engine does not yet
accept, and for up to a consumer's own cache lifetime afterwards a cached
reader keeps presenting one the engine no longer accepts. With a single
credential no write ordering avoids this; only overlapping validity (two
alternating users) removes it, and formae does not orchestrate that today.

So when a user asks for rotation on a credential that something reads to
authenticate, state the consumer contract out loud:

- **The consumer must re-read the credential** at least once per rotation
  period, per connection or on a bounded cache TTL. A consumer that reads it
  once at startup breaks on the first rotation and stays broken until restarted.
- **The consumer must tolerate transient authentication failures around a
  rotation**, for up to its own cache lifetime, by retrying or reconnecting.
  The failures end on their own when its cache expires.

The formae agent is itself a worked example of the contract: pointing
`datastore.postgres.passwordSecretArn` at the secret makes it resolve its own
database password per new connection, and it rides out a rotation of that
credential without a restart.

## Inspecting rotation state

Use `list_generators` (pass an explicit `profile`): each generator's label,
type, stack, cadence in seconds, the instant of its last committed rotation,
and the resources bound to it. Secret values are never returned.

Gotchas:

- **Rotations do not appear in `list_commands`**, because they run agent-side.
  To answer "when did X last rotate?", use `list_generators`, not the command
  history.
- A generator with `LastRotatedAt` unset has not rotated since creation; that
  is normal for long cadences, not a failure.
- Destroying a stack does not currently take its generators with it. If the
  user tears down a stack with generators, mention that the generators remain
  and point them at `list_generators` to verify.

## Simulate first

`apply_forma` with `simulate=true` shows the generator operation, whether a
draw is impending, and which destinations it moves, with no material. An apply
that plans no rotation says so, which makes "nothing will rotate" an assertion
rather than an absence.

For deeper coverage link the user to the secrets concept page from
`formae://docs/index` rather than constructing a URL.
