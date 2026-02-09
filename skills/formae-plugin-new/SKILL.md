---
name: formae-plugin-new
description: "Use when the user wants to create a new formae resource plugin, build a provider integration, or extend formae with support for a new cloud service"
---

# Build a New Resource Plugin

Create a complete formae resource plugin for a cloud provider or service, following the plugin SDK tutorial and using TDD for the implementation.

## MANDATORY RULE: Install Before Testing

After ANY code changes to the plugin, you MUST run `make install` before running conformance tests. The conformance tests run against the INSTALLED plugin binary, not the source code. Skipping this step means you're testing stale code.

## Workflow

### 1. Gather requirements

Collect the following from the user:

- **Provider/service**: e.g., Cloudflare, Datadog, GitHub
- **Resource types**: e.g., DNS records, monitors, repositories
- **Credentials**: how should authentication be configured? (env vars, config files, API keys)
- **Polymorphic structures**: are there resources with different field shapes per subtype? (e.g., DNS records with different fields per record type)
- **Development mode**: autonomous or guided?
  - **Autonomous**: TDD through the entire implementation without pausing for review — for one-shotting a plugin quickly
  - **Guided**: pause at checkpoints for user review before continuing — for production-grade plugins where the user wants to curate schema design and review each step

### 2. Research the provider API

Research the provider's API to understand:
- Resource models: required vs optional fields, immutable fields (createOnly), read-only fields
- CRUD operations: endpoints, request/response shapes, error codes
- Authentication: how credentials are passed
- Async operations: does the API return immediately or require polling?
- Value normalization: does the API normalize values? (e.g., FQDN vs short name, case changes, default values injected server-side)

Look at how Terraform or Pulumi model equivalent resources for schema design reference.

If resources have polymorphic structures (like DNS record types with different field shapes), design the schema to accommodate all variants.

**Guided mode checkpoint**: present API research findings and proposed resource model to the user for approval before continuing.

### 3. Scaffold the plugin

Run the scaffolding command:

```
formae plugin init --no-input --name <name> --namespace <NS> --description "<desc>" --author "<author>" --module-path "<path>" --license Apache-2.0
```

Verify the scaffolded project:
- Check the directory structure: `ls -R`
- Check the Makefile and `formae-plugin.pkl`
- Confirm it builds: `make build`

Then `cd` into the new plugin directory and run `/init` so Claude has full context on the scaffolded template before continuing.

### 4. Follow the tutorial

Fetch the plugin SDK tutorial from `https://docs.formae.io/en/latest/plugin-sdk/tutorial/` and follow it start to finish, adapting each lesson to the target provider.

Reference prior art plugins for real-world patterns:
- AWS: `https://github.com/platform-engineering-labs/formae-plugin-aws`
- Azure: `https://github.com/platform-engineering-labs/formae-plugin-azure`
- GCP: `https://github.com/platform-engineering-labs/formae-plugin-gcp`
- OCI: `https://github.com/platform-engineering-labs/formae-plugin-oci`
- OVH: `https://github.com/platform-engineering-labs/formae-plugin-ovh`

Tutorial lessons to follow (adapt to target provider):

1. **Schema** — define PKL resource types with annotations (`@ResourceType`, `@CreateOnly`, `@ReadOnly`)
2. **Target configuration** — set up provider authentication and connection settings
3. **Plugin configuration** — RateLimit, LabelConfig, DiscoveryFilters
4. **Create** — implement resource creation
5. **Read** — implement resource reading (handle NotFound correctly)
6. **Update** — implement resource updates
7. **Delete** — implement resource deletion (handle NotFound as success)
8. **List** — implement resource discovery
9. **Error handling** — use `OperationErrorCode` patterns from the SDK
10. **Conformance tests** — run `make conformance-test`

**Use TDD for steps 4–8**: write a failing test, implement the minimum code to pass it, verify, then move to the next operation.

**Guided mode checkpoints** (skip all in autonomous mode):
- After step 1 (schema): present PKL resource definitions for review
- After step 2 (target): present target config approach for review
- After each CRUD operation (steps 4–8): present the test and implementation for review
- After step 8 (list/discovery): present discovery implementation for review

In guided mode, do not proceed past a checkpoint until the user approves. If the user requests changes, make them and re-present.

### 5. Conformance tests

Run `make install && make conformance-test`.

Fix any failures and re-run. All CRUD and discovery tests must pass before continuing.

### 6. Local end-to-end test

Create a test project and verify the full lifecycle:

1. `formae project init` in a temporary directory
2. Write a simple forma file using the new plugin's resource types
3. `formae apply --mode reconcile --simulate` to verify the plan
4. `formae apply --mode reconcile` to create real resources
5. `formae inventory` to verify resources are managed
6. `formae destroy` to clean up

### 7. Examples and documentation

- Add a working example in the `examples/` directory
- Update `README.md` with:
  - Target configuration instructions
  - Credentials setup
  - Supported resource types with descriptions
  - Example usage

### 8. Definition of Done

Before reporting completion, verify each item:

- [ ] All target resource types are implemented with full CRUD
- [ ] Conformance tests pass (`make install && make conformance-test`)
- [ ] Local end-to-end test passes (create, read, update, destroy)
- [ ] Working example in `examples/`
- [ ] README has: target config, credentials, supported resources, examples

## Important

- After ANY code changes, ALWAYS run `make install` before `make conformance-test` — tests run against the installed binary
- "Plugin not found" errors mean the plugin isn't installed or is at the wrong path — run `make install` and check the output
- Cloud APIs often normalize values (FQDN vs short name, case changes, default values) — preserve user input where possible and normalize for idempotency where the API requires it
- NEVER skip conformance tests — they verify the full CRUD lifecycle through the formae agent
- NEVER skip the local end-to-end test — conformance tests alone don't verify the full integration
- NEVER ask the user to run tests — run them yourself and report results
