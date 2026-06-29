---
name: formae-config
description: "Use when the user wants to switch, list, save, edit, delete, or compare formae configuration profiles in ~/.config/formae/"
---

# formae-config

formae configuration profiles are named configs under the formae config dir
(`~/.config/formae/profiles/<name>.pkl`), with the active one recorded in a
plain pointer file (`<config-dir>/active`). They are managed by the
`formae profile` subcommand (formae >= 0.87.0) and exposed through the
formae-mcp server as native tools. The old stand-alone `fcfg` binary is gone.

Switching the active profile takes effect for subsequent MCP calls immediately —
no agent or MCP-server restart needed.

## MCP tools

| Intent | Tool | Notes |
|--------|------|-------|
| List profiles + active | `list_profiles` | returns `{"active": "<name>", "profiles": ["..."]}` |
| Show active | `current_profile` | returns `{"active": "<name>"}` |
| Switch active (default env) | `use_profile` | `{ "name": "<name>" }` — global; only on explicit "change my default" requests (see below) |
| Snapshot active under a new name | `save_profile` | `{ "name": "<name>", "force": false }` (does not switch) |
| Create from template | `create_profile` | `{ "name": "<name>", "force": false }` (does not switch) |
| Delete | `delete_profile` | refuses the active profile — switch first |
| Compare | `diff_profiles` | `{ "a": "<name>", "b": "<name>?" }` (b defaults to active) |
| View PKL | `read_profile` | `{ "name": "<name>" }` returns the profile's PKL |
| Replace PKL | `write_profile` | `{ "name": "<name>", "content": "<pkl>" }` |

All profile tools require formae >= 0.87.0; on an older formae they return
`requires formae >= 0.87.0 (connected: A.B.C)`.

## Editing a profile

There is no interactive `$EDITOR` step. To modify a profile:

1. `read_profile` to fetch its current PKL.
2. Edit the PKL.
3. `write_profile` to replace it.

`write_profile` is **overwrite-only** (use `create_profile` for a new profile) and
**refuses the active profile** — to edit the active one, `use_profile` to switch
away first (or `save_profile` a copy, edit the copy, then switch). Content is
written as-is; formae has no config validator, so a malformed profile surfaces at
the next `use_profile`/apply rather than at write time.

## Targeting an environment — prefer `profile`, don't switch active

**Default to the per-invocation `profile` argument; do not switch the active
profile to set up a session.** Every agent-touching tool (apply / destroy /
status / inventory / list_* / force_* / cancel / extract) accepts an optional
`profile` argument that targets that one call only and changes no global state.

This matters because the active profile is **global, persisted state** shared with
the user's `formae` CLI and every other concurrent session. Multiple sessions may
be working against different agents at the same time, so calling `use_profile` to
"point this session at staging" would silently redirect those other sessions too.
Always pass `profile` per call instead.

Only call `use_profile` when the user **explicitly** asks to change their default
environment/agent (e.g. "make prod my default") — never as a per-session setup step.

The plugin-hub tools (`search_hub_plugins`, `get_hub_plugin`,
`list_plugin_examples`, `get_plugin_example`) and `create_inline_policy` do **not**
accept `profile` (they don't talk to a specific agent) — don't pass it to them.

## Direct CLI (when not using the MCP)

- `formae profile list --output-consumer machine --output-schema json`
- `formae profile current --output-consumer machine --output-schema json`
- `formae profile use <name>` / `create <name>` / `save <name> [--force]` /
  `delete <name>` / `diff <a> [<b>]`

## Worked example

User: "switch my formae to load-test"

1. Call `use_profile` with `{ "name": "load-test" }`.
2. On a version error: tell the user they need formae >= 0.87.0.
3. On "profile not found": call `list_profiles` to show what's available.
4. On success: confirm. No restart needed — subsequent tools use load-test.
