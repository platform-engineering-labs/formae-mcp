---
name: setup
description: Use to get formae working in this environment — signs in to the hosted platform, or sets up a self-hosted profile, then checks the agent is reachable.
---

# Set up formae

Gets a machine from "nothing configured" to "you can work with your
infrastructure", whichever way the user runs formae.

**Do the steps in order.** Each one answers a question the next depends on, and
the ordering is what keeps this skill from tripping the same no-profile prompt it
exists to resolve: steps 1 to 5 touch no agent-backed tool, so none of them needs
a working agent or an existing profile.

## Step 1 — Is the plugin wired up, and what does this machine have?

Run `list_profiles`.

It is a pure read that needs no agent and no profile, and it answers both
questions at once: if it returns, the plugin is working, and what it returns is
what this machine holds.

Do **not** use `check_health` for this. It also requires a reachable agent, so on
a machine that has none it reports a failure that is not about setup at all.

For Codex and OpenCode, also confirm the MCP server entry exists in the harness
config and add it if missing.

## Step 2 — Branch on what you found

- **Profiles already exist** → skip to step 5. This machine is configured; the
  user is here to check it, or to sign in again.
- **No profiles** → ask the user which they want, and wait for the answer:

  > Are you using the **hosted** formae platform, or running **your own agent**?

Do not guess. The two paths are different and nothing on the machine says which
they meant.

## Step 3 — Self-hosted

Call `use_profile` with `name: "default"`.

That creates the default profile if it does not exist and makes it active.

Then tell them what they have, in one or two sentences:

- the profile points at a local agent on `http://localhost:49684` by default;
- `formae profile edit` opens it, to point at an agent somewhere else;
- offer to help them author their first forma, or to look at infrastructure they
  already have.

Stop here. There is nothing to sign in to on a self-hosted setup, and offering it
would be wrong.

## Step 4 — Hosted

If they do not have an account yet, send them to
[console.formae.ai](https://console.formae.ai) to create one, and wait until they
say it is done.

Then:

1. Call `login`. If the user has told you they have no browser on this machine,
   pass `device: true`.
2. **Show them exactly what it returns** — the URL to open, or the verification
   URL and the code to type. This is the whole point of the step; they cannot
   continue without it.
3. Wait for them to say they have finished.
4. Call `complete_login`.

If `login` reports that they are already signed in, that is a success: it means
a valid session was already open. Carry on to step 5.

## Step 5 — Check what was written

Run `list_profiles` again and tell the user which profile is now active.

A sign-in that created no profiles is a real outcome, not a failure to report
around: it usually means the account covers no installations yet. Say so plainly
and point them back to the console rather than continuing as though it worked.

## Step 6 — Now check the agent

Run `check_health`.

This is the first step that needs a reachable agent, which is why it is last.

- On a version-skew notice, hand off to `/formae:upgrade`.
- If it fails on a self-hosted setup, the likely cause is that no agent is
  running yet, or the profile points at the wrong address — not that setup went
  wrong. Say which, and offer `formae profile edit`.

Then tell them they can work with their infrastructure, and offer a first step:
listing what they have, or authoring something new.
