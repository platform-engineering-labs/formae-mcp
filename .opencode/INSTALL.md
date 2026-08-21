# Installing the formae MCP for OpenCode

## Prerequisites

- Git
- `curl`
- OpenCode
- A running formae agent (`formae agent start`) and a formae profile pointing at it

You do **not** need a Go toolchain. The MCP server and a matched `formae` binary
are downloaded as prebuilt artifacts on first launch — nothing is compiled on your
machine.

## Installation

1. Clone the repo (it carries the skills and the launcher):

   ```bash
   git clone https://github.com/platform-engineering-labs/formae-mcp.git ~/.config/opencode/formae
   ```

2. Symlink the skills into OpenCode:

   ```bash
   mkdir -p ~/.config/opencode/skills
   ln -s ~/.config/opencode/formae/skills ~/.config/opencode/skills/formae
   ```

3. Register the MCP server. Point OpenCode at the launcher script, which
   downloads the prebuilt `formae-mcp` (and a matched `formae`) into
   `~/.formae-ai/opt` on first run and starts the server. Merge this into
   `~/.config/opencode/opencode.json` (add to your existing config — don't
   overwrite other keys):

   ```json
   {
     "$schema": "https://opencode.ai/config.json",
     "mcp": {
       "formae": {
         "type": "local",
         "command": ["/home/you/.config/opencode/formae/scripts/start-mcp.sh"],
         "enabled": true
       }
     }
   }
   ```

   Use an **absolute** path to `start-mcp.sh` (expand `~`), and make sure it is
   executable (`chmod +x`). No binary needs to be on your `PATH`.

4. Restart OpenCode.

## Verify

Two checks — the first confirms OpenCode sees the server, the second confirms it
actually works end-to-end.

1. Confirm the server is registered and connected:

   ```bash
   opencode mcp list
   ```

   `formae` should appear in the list.

2. With a formae agent running (`formae agent start`), ask OpenCode for formae
   status (invoke the `formae-status` skill, e.g. "what formae commands are
   running?") and confirm a tool call returns live agent data — not just that
   the skill loaded.

## Updating

Pull the latest skills and launcher:

```bash
cd ~/.config/opencode/formae && git pull
```

On the next launch, if the plugin version changed, the launcher automatically
downloads the matching `formae-mcp` binary. To update the `formae` binary when the
connected agent is newer, run the `/formae:upgrade` skill (it asks first).

## Uninstalling

Delete the `mcp.formae` block from `~/.config/opencode/opencode.json`, then
remove the skills symlink and clone:

```bash
rm ~/.config/opencode/skills/formae
rm -rf ~/.config/opencode/formae
```

Optionally remove the downloaded binaries:

```bash
rm -rf ~/.formae-ai/opt
```
