---
name: setup
description: Use to get formae working in this environment — signs in to the hosted platform, or sets up a self-hosted profile, then checks the agent is reachable.
---

# Set up formae

Gets a machine from "nothing configured" to "you can work with your
infrastructure", whichever way the user runs formae.

**Do the steps in order.** Each one answers a question the next depends on, and
the ordering is what keeps this skill from tripping the same no-profile prompt it
exists to resolve: steps 1 to 4 touch no agent-backed tool, so none of them needs
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

**This skill assumes hosted. Do not ask.**

`/formae:setup` is advertised in one place: the hosted console, which tells a user
to run it after creating an account. Someone running it therefore came from there.
Asking which of the two they are on makes a user answer a question their own last
click already answered. (The question is not written out anywhere in this file on
purpose: a quoted question is a thing a model skimming for its next action can
end up asking.)

That is not true of anything else. A user who reaches for some other tool with no
profile on disk genuinely could be either, and the MCP asks them — the question
lives there, where the ambiguity is real, and not here.

- **No profiles** → go to step 3 and sign them in.
- **Profiles exist, and one is active** → skip to step 4. This machine is
  configured; they are here to check it, or to sign in again.
- **Profiles exist but none is active** → `list_profiles` shows them with no
  active marker. Ask which one to use, call `use_profile` with it, then go to
  step 4. Do not skip this: with no active profile every agent-backed tool
  refuses, so `check_health` in step 5 would fail for a reason that has nothing
  to do with the agent.

The one exception: if the user *volunteers* that they self-host, believe them and
go to step 6 instead. Do not prompt for it.

## Step 3 — Hosted

If they do not have an account yet, send them to
[console.formae.ai](https://console.formae.ai) to create one, and wait until they
say it is done.

Then:

1. **Decide the flow before you start one.** A browser sign-in redirects to
   `127.0.0.1` on *this* machine, so it only works where a browser can reach
   that. Check cheaply first:

   ```sh
   [ -f /.dockerenv ] && echo container; echo "DISPLAY=${DISPLAY:-unset} SSH=${SSH_CONNECTION:-none}"
   ```

   A container, an SSH session, or no `DISPLAY` on Linux all mean no local
   browser: pass `device: true`. Otherwise start the browser flow and say that
   they should tell you if they have no browser here.

   Getting this wrong costs a round trip — the first flow is abandoned and a
   second one started, and the code from the first no longer works.

2. Call `login`.
3. **Show them exactly what it returns** — the URL to open, or the verification
   URL and the code to type. This is the whole point of the step; they cannot
   continue without it.
4. Wait for them to say they have finished.
5. Call `complete_login`.

If `login` reports that they are already signed in, that is a success: it means
a valid session was already open. Carry on to step 4.

If `complete_login` reports that they are **signed in but the profiles could not
be written**, do not start another sign-in. Their session is saved and a second
one fixes nothing; the problem is between formae and the control plane. Report it
as the message says, and offer `formae login --hosted` for the reason.

## Step 4 — Check what was written

Run `list_profiles` again and tell the user which profile is now active.

A sign-in that created no profiles is a real outcome, not a failure to report
around: it usually means the account covers no installations yet. Say so plainly
and point them back to the console rather than continuing as though it worked.

## Step 5 — Now check the agent

Run `check_health`.

This is the first step that needs a reachable agent, which is why it is last.

- On a version-skew notice, hand off to `/formae:upgrade`.
- If it fails on a self-hosted setup, the likely cause is that no agent is
  running yet, or the profile points at the wrong address — not that setup went
  wrong. Say which, and offer `formae profile edit`.

Then tell them they can work with their infrastructure, and offer a first step:
listing what they have, or authoring something new.

## Step 6 — Self-hosted, only if they said so

Reached only when the user volunteered that they run their own agent.

Call `use_profile` with `name: "default"`, which creates the default profile if it
does not exist and makes it active. Then, in a sentence or two: the profile points
at a local agent on `http://localhost:49684`, `formae profile edit` repoints it,
and offer to help author a first forma.

There is nothing to sign in to on a self-hosted setup, so do not offer it.
