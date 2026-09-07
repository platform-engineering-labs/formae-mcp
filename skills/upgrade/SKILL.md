---
name: upgrade
description: Use to upgrade the local formae when the connected agent is newer (classic mode) — always asks first and warns that it may move a pinned formae.
---

When a version-skew notice reports the agent is newer than local formae:
1. Show the user the current vs target versions and warn that upgrading may move a formae they pinned to match their own agent.
2. Only on explicit confirmation, run the formae installer to upgrade.
3. Re-run `check_health` to confirm the skew is resolved.
Never upgrade without confirmation in classic mode.
