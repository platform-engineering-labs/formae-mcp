---
name: formae-stack-design
description: "Use when the user is deciding how to group resources into stacks, where to draw stack boundaries, or how stacks/targets/policies relate — stack design and reconciliation-boundary guidance."
---

# Design Formae Stacks

Help the user decide how to group resources into stacks, where to draw boundaries, and how stacks, targets, and policies relate. This is a **guidance skill** — it does not mutate infrastructure. It results in a proposed design that the user then authors.

Before advising, read `formae://docs/stack-design` for the canonical reference. Also read `formae://docs/forma-structure` if the user is about to start authoring files.

---

## Core model: stack = reconciliation boundary

**The most important fact about stacks:** `apply --mode reconcile` makes the stack match its PKL exactly. It will **delete** any resource in the stack that is not declared in the PKL. The stack is the blast radius of one reconcile.

Ask the user:

- Which resources change together? Which are long-lived vs. ephemeral?
- Which resources are "protected" (production databases) vs. easy to recreate?
- Are there resources with very different risk levels that should not share a blast radius?

**Rule of thumb:** don't put resources with very different change cadences or risk levels in the same stack.

---

## Stacks are orthogonal to targets

A **target** is a cloud account or endpoint (e.g., an AWS account, a Kubernetes cluster, a Grafana instance). A **stack** is a reconcile group. These two concepts are independent:

- One stack can span multiple targets (e.g., an AWS resource and a k8s resource that belong together).
- One target's resources can be split across multiple stacks (e.g., a shared EKS cluster in one stack; per-service k8s deployments in another).

Resolvables (cross-resource references) can cross both stack and target boundaries — so don't let target boundaries force your stack grouping.

---

## Nested/recursive targets and multi-plugin wiring

Targets can chain: the config of one target can reference a resource from another target via a resolvable. This is how multi-plugin stacks wire together. Classic example:

1. An **AWS target** manages an EKS cluster (`appCluster`).
2. A **Kubernetes target** reads `appCluster.res.endpoint` and `appCluster.res.certificateAuthority` to configure its API server connection.
3. A **Grafana target** reads the in-cluster Grafana service URL from a k8s Service resource to connect to Grafana's API.

Each target declares its resources, but the targets themselves form a dependency chain. This chaining is expressed via resolvables in PKL — not by merging everything into one stack.

To see a real example of this pattern, offer to pull one via `list_plugin_examples` (e.g., `lgtm-observability` or `bookstore`):

> "I can pull the `lgtm-observability` example with `list_plugin_examples` to show how the AWS → k8s → Grafana target chain is wired in real PKL. Want me to do that?"

---

## Apply ordering follows resolvable edges

If Stack B's resources reference a resource in Stack A via a resolvable, Stack A must be applied first. When proposing a multi-stack split, always call this out explicitly:

- State which stacks are **producers** (depended on) and which are **consumers** (depend on producers).
- State the apply order: `apply stack-a` → `apply stack-b`.

This ordering is the user's responsibility — formae does not automatically sequence multi-stack applies.

---

## Policies attach per stack

Two policy types are available on a stack, and both reinforce the "group by lifecycle" principle:

- **TTL** — destroys the stack after a duration. Use for ephemeral environments (preview branches, short-lived test clusters). If ephemeral resources live alongside long-lived ones, the TTL would destroy the long-lived ones too.
- **Auto-reconcile** — periodically reverts out-of-band changes. Use for protected stacks (production) where drift should never persist. You may not want this on a development stack where manual tweaks are common.

These policies are another strong reason to split by lifecycle and protection level. Point the user to the `formae-policy` skill when they are ready to author policies.

---

## Guidance workflow

Walk through these questions with the user:

1. **Inventory intent.** What resources are involved? What plugins/targets? (If unknown, offer to call `list_resources` or `list_targets` to see what's already there.)

2. **Group by change cadence and risk.**
   - Fast-changing / low-risk: development services, preview envs → own stack, TTL policy candidate.
   - Slow-changing / high-risk: shared infrastructure (VPCs, clusters, databases) → own stack, auto-reconcile policy candidate.
   - Everything else: evaluate by blast radius.

3. **Identify cross-stack references.** Any resource that another stack depends on becomes a producer. Map these edges and confirm the apply order.

4. **Check target boundaries.** Does the split respect or cut across target boundaries? Both are valid — confirm intentionality.

5. **Sketch the proposed split.** Before the user authors anything, offer to lay out:
   - Stack names and which resources belong to each.
   - Which targets each stack touches.
   - Cross-stack resolvable edges (A.res.x → B).
   - Recommended apply order.
   - Policy candidates per stack.

6. **Hand off to authoring.** Once the design is agreed, reference `formae://docs/forma-structure` for file layout conventions and offer to continue with `formae-project-init` (new project) or `formae-patch` / `formae-apply` (existing project).

---

## CONSTRAINTS

- **This skill does not apply or mutate infrastructure.** It guides design only.
- **Never propose a stack split without calling out apply ordering** when cross-stack resolvables exist.
- **Never merge unrelated lifecycle tiers into one stack** — always flag the blast-radius consequence.
- **Always read `formae://docs/stack-design`** before advising; the canonical content there overrides any heuristic above.
- **Point to `formae-policy`** when TTL or auto-reconcile details come up — don't duplicate that skill's workflow here.
