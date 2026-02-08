---
name: formae-plugins
description: "Use when the user asks about installed formae plugins, supported cloud providers, available resource types, or plugin versions"
---

# List Active Plugins

Use the `list_plugins` MCP tool to show active formae plugins.

## Workflow

1. Call `list_plugins` (no parameters needed)
2. Present plugins organized by type: resource plugins, schema plugins, network plugins
3. Show plugin name, namespace, version, and capabilities

## Plugin Types

- **Resource plugins**: Manage cloud resources (e.g., AWS, Azure, GCP)
- **Schema plugins**: Parse infrastructure declarations (PKL, JSON, YAML)
- **Network plugins**: Handle networking (e.g., Tailscale)
