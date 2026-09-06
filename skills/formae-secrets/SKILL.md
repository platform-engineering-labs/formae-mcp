---
name: formae-secrets
description: "Use when the user wants formae to manage a credential — generate a password, rotate a secret, wire a database password or key pair, ask what rotates or when something last rotated — e.g. 'generate the db password', 'rotate this every 90 days', 'is this secret managed by formae?', 'when did the identity key last rotate?'"
---

# Secrets and Generators

Use this skill to author generator-backed credentials and to inspect rotation
state. Requires formae >= 0.89.0 (the `list_generators` tool refuses on older
versions; treat that refusal as "this formae predates generators").

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
not promise the value is fixed for life: an apply can still redraw it — when a
new destination binds to the generator, or when the generator's spec changes
so the held generation no longer satisfies it. The consumer contract below
applies to those redraws too. Key pairs: `formae.KeyPairGenerator` draws an
RSA pair as two named outputs of one draw — bind `gen.privateKey` (PKCS#8 PEM)
and `gen.publicKey` (PKIX PEM) to their own destinations and the two always
hold halves of the same pair.

**Do NOT author the legacy shape** (eval-time `random.password(...)` seeded
into a `.opaque.setOnce` secret). It cannot rotate, and `setOnce` is a one-way
door: a resource created on that shape can never adopt a generator without
being destroyed and recreated.

## Choosing a cadence

- Steady state: days. 30 or 90 days (`30.d`, `90.d`) are the common choices.
- The schema floor is 15 minutes, set by what the common secret store
  sustains (AWS Secrets Manager retains every version from the last 24 hours
  against a 100-version quota). Treat minutes-scale cadences as drill-only.
- A credential that must turn faster than the floor wants short-lived
  per-use credentials, not a scheduler revisiting a long-lived one.

State the chosen cadence to the user before applying, and simulate before
applying as always.

## What a rotation does, and what consumers must tolerate

A rotation is one coordinated command: the generator's destinations (the
secret) and, transitively, the resources that consume them by reference all
move together. Rotation uses ordinary update semantics, so it refuses when the
stack or any consumer has drifted; a stack carrying an auto-reconcile policy
is the standing opt-in to overwrite drift, and there rotation proceeds.

Warn the user about the consumer contract: no rotation scheme with a single
credential can remove the brief window where the store and the accepting
system disagree. Anything that authenticates with the secret must re-read it
at least once per rotation period (resolve per connection, or cache with a
bounded lifetime) and tolerate transient auth failures around a rotation. A
consumer that reads the credential once at startup breaks on the first
rotation and stays broken until it restarts.

## Inspecting rotation state

Use `list_generators` (pass an explicit `profile`): each generator's label,
type, stack, cadence in seconds, the instant of its last committed rotation,
and the resources bound to it. Secret values are never returned.

Gotchas:

- **Rotations do not appear in `list_commands`** — they run agent-side. To
  answer "when did X last rotate?", use `list_generators`, not the command
  history.
- A generator with `LastRotatedAt` unset has not rotated since creation;
  that is normal for long cadences, not a failure.
- Destroying a stack does not currently take its generators with it; if the
  user tears down a stack with generators, mention the generators remain and
  point them at `list_generators` to verify.

For deeper coverage link the user to the secrets concept page from
`formae://docs/index` rather than constructing a URL.
