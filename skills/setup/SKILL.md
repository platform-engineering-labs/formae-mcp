---
name: setup
description: Use to install or repair the formae MCP for this project — ensures the prebuilt binary and bundled formae are present and the MCP is registered in the current harness.
---

Ensure formae is ready in this environment:
1. Confirm the plugin's prebuilt binary launches (run the MCP `check_health` tool).
2. If a version-skew notice appears, hand off to `/formae:upgrade`.
3. For Codex/OpenCode, confirm the MCP server entry exists in the harness config; add it if missing.
