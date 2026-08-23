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

`/formae:setup` is advertised in one place: the hosted console. Someone running it
therefore came from there, and asking which of the two they are on makes a user
answer a question their own last click already answered. (The question is not
written out anywhere in this file on purpose: a quoted question is a thing a model
skimming for its next action can end up asking.)

The one reader this does not describe is someone who came here *before* the
console, which is the case step 4 exists to close. They are still hosted, so the
assumption holds; they just take the longer way round.

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
go to step 7 instead. Do not prompt for it.

## Step 3 — Hosted

**Sign in first. Do not ask whether they have an account, and do not send them
anywhere yet.**

An invitation is claimed at the invited user's first sign-in, so someone who has
never heard of formae's console may still finish `login` with a working agent
already waiting for them. Sending them to the console first costs them a trip
they never needed. And for a user who genuinely has no organization, signing in
is not wasted either: the session it opens is what makes the second pass in step
4 cheap.

So:

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

## Step 4 — Check what was written, and finish the round trip

Run `list_profiles` again.

**A profile exists and one is active** → tell them which, and go to step 5.

**No profiles were written.** This is a real outcome, not a failure to report
around: it means their account covers no installations yet, so there is no agent
to point a profile at. Do not treat it as a sign-in failure and do not start
another sign-in; the session is fine, they just have nothing to connect to.

Finish the trip rather than ending on it:

1. Send them to
   [console.formae.ai](https://console.formae.ai/?from=mcp) to create an
   organization and provision an agent. **Use that link, with the `?from=mcp` on
   it.** It tells the console the reader came from here, so it stops closing
   their signup by telling them to run `/formae:setup` — which is what they are
   already inside — and points them back to this session instead.
2. Wait until they say the agent is set up. It takes about a minute.
3. Call `login` again.

   There is normally no second browser hop: the session from step 3 is still
   open, so `login` reports that they are already signed in and goes straight on
   to enumerating what their account now covers, which is what writes the
   profile. It returns that result itself.

   **Only call `complete_login` if this `login` hands you a URL or a device
   code.** That means the session lapsed and a real flow started, so show it to
   them and wait, exactly as in step 3. Calling `complete_login` after a `login`
   that already finished is an error: there is no sign-in in progress for it to
   complete, and it will say so.
4. Run `list_profiles` once more.

**If the second pass still writes no profiles, stop and say so.** Do not loop
again on your own. Report what you found and let them decide; asking the same
question a third time only spends their time. `formae login --hosted` in a
terminal is worth offering at that point, because its own output says more about
why than this tool surfaces.

## Step 5 — Now check the agent

Run `check_health`.

This is the first step that needs a reachable agent, which is why it is last.

- On a version-skew notice, hand off to `/formae:upgrade`.
- If it fails on a self-hosted setup, the likely cause is that no agent is
  running yet, or the profile points at the wrong address — not that setup went
  wrong. Say which, and offer `formae profile edit`.

## Step 6 — Is there a cloud account to manage?

A reachable agent is not the finish line: a hosted installation with no cloud
account registered cannot manage anything yet. Before telling them the journey
is done, run `list_cloud_connections`.

- **Registered connections exist.** Name each one (cloud and account). Say
  they are **registered**, never verified, working, or that formae can manage
  them: this reports what the control plane has on file, not that the role
  has been used successfully.
- **The listing completed and came back empty.** No cloud account is
  registered yet. Say so, and offer to connect one. AWS is the only cloud
  `formae connect` has a subcommand for, so offer that directly rather than
  presenting a menu of clouds that mostly dead-end.

  An affirmative answer is not enough to act on: the account id is still
  needed. Do not hand off and stop here; continue straight into the
  `/formae:connect` flow, which opens by asking for the 12-digit AWS account
  id.
- **The listing did not complete, or the tool failed.** Say that whether a
  cloud account is registered could not be determined, and stop there. Do
  **not** offer to connect one on this basis: an unreadable answer is not the
  same as "none registered", and offering to provision because a response
  could not be read is exactly the mistake this check exists to prevent.

Only once a cloud account is confirmed registered (found already, or just
connected) tell them they can work with their infrastructure, and offer a
first step: listing what they have, or authoring something new.

## Step 7 — Self-hosted, only if they said so

Reached only when the user volunteered that they run their own agent.

Call `use_profile` with `name: "default"`, which creates the default profile if it
does not exist and makes it active. Then, in a sentence or two: the profile points
at a local agent on `http://localhost:49684`, `formae profile edit` repoints it,
and offer to help author a first forma.

There is nothing to sign in to on a self-hosted setup, so do not offer it.
