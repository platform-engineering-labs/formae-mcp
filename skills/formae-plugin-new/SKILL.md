---
name: formae-plugin-new
description: "Use when the user wants to create a new formae resource plugin, build a provider integration, or extend formae with support for a new cloud service"
---

# Build a New Resource Plugin

Create a complete formae resource plugin for a cloud provider or service, following the plugin SDK tutorial and using TDD for the implementation.

## When you arrive here from authoring

Building a resource plugin is a **large, multi-step, context-hungry effort**: provider API research, schema design, CRUD implementation, TDD for every operation, conformance tests, and end-to-end verification. The full workflow (steps 1–8 below) easily spans hundreds of tool calls and thousands of tokens — far bigger than authoring a forma file.

**If the user arrived here mid-authoring** (i.e., they were writing a forma file and discovered no plugin exists for their provider), offer to run the plugin build as a **separate sub-agent or fresh session** before diving in:

> "Building this plugin is a large task that will likely fill the current conversation's context window. I recommend starting a fresh Claude Code session (or I can launch a sub-agent) so the plugin build doesn't crowd out your authoring work. Want me to proceed that way?"

**If the user is not actively managing context**, proactively warn them before starting:

> "Heads up: building a full plugin typically uses 40–60 % of a context window on its own. I'll warn you again at 40 % and every 10 % after that, but if you'd prefer to keep this conversation focused on authoring, I can hand the plugin work to a sub-agent now."

Only skip these prompts if the user has already confirmed they want to build the plugin in the current session.

## MANDATORY RULE: Install Before Testing

After ANY code changes to the plugin, you MUST run `make install` before running conformance tests. The conformance tests run against the INSTALLED plugin binary, not the source code. Skipping this step means you're testing stale code.

## Workflow

### 1. Gather requirements

Ask the user an open-ended question: what provider or service do they want to build a plugin for, and which resource types should it support? Do NOT present a multiple-choice list of providers — there are thousands of possible targets and we cannot guess. Just ask.

Collect the following from the user:

- **Provider/service**: e.g., Cloudflare, Datadog, GitHub
- **Resource types**: Once the user names the provider, research the provider's API thoroughly before suggesting resource types. Present a well-researched list organized in implementation waves (e.g., "Wave 1: core resources, Wave 2: networking, Wave 3: IAM/security"). The user should be able to pick individual resources or entire waves. Do NOT present a hastily assembled shortlist — take the time to understand the provider's full resource catalog first.
- **Credentials**: how should authentication be configured? (env vars, config files, API keys)
- **Namespace convention**: The provider namespace becomes the uppercase prefix in formae resource type IDs (e.g., `AWS`, `GCP`, `ATLAS`). The scaffolder will uppercase mixed-case input automatically and emit a notice when it does so.
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

### 2a. External binary dependencies

If your plugin shells out to an external binary (e.g., `atlas`, `helm`, `kubectl`), the binary must be available in the formae agent's `$PATH` at runtime. The plugin SDK does not yet ship a first-class mechanism for bundling extra binaries with plugin distributions; the conventions below are the current stop-gap.

**Prior art.** Formae core ships the `pkl-reader-helm` external binary using the pattern at `formae/scripts/install-helm-reader.sh`. This is the canonical reference for any plugin that depends on a runtime binary — read it before designing your own.

**Plugin-side convention.** On plugin initialization, call `exec.LookPath("<binary>")` and fail fast with a clear error message naming both the binary and the install path. Example error:

```
plugin atlas: required binary `atlas` not found in $PATH.
Install with: /path/to/install-atlas.sh
See: https://atlasgo.io/getting-started
```

Optionally expose a health-check endpoint that reports binary availability so operators can diagnose deployment issues without invoking real CRUD operations.

**Multi-arch handling.** Mirror the `uname -s` / `uname -m` mapping pattern from `formae/scripts/install-helm-reader.sh` to select the correct release asset per platform.

**Dev-mode bootstrap.** Until the SDK supports bundled binaries, mirror the install script in your plugin's own repository (e.g., `scripts/install-atlas.sh`). Have `make install` invoke it so fresh dev machines and CI runners can bootstrap automatically.

**Known SDK limitation.** Track the follow-up "plugin ships extra binaries" SDK enhancement — when that lands, plugins will declare their binary dependencies in `formae-plugin.pkl` and the agent host will install them at plugin registration time.

### 3. Scaffold the plugin

Run the scaffolding command:

```
formae plugin init --no-input --name <name> --namespace <NS> --description "<desc>" --author "<author>" --module-path "<path>" --license Apache-2.0
```

Verify the scaffolded project:
- Check the directory structure: `ls -R`
- Check the Makefile and `formae-plugin.pkl`
- Confirm it builds: `make build`

### 4. Follow the tutorial

Fetch the plugin SDK tutorial from `https://docs.formae.io/en/latest/plugin-sdk/tutorial/` and follow it start to finish, adapting each lesson to the target provider.

Reference prior art plugins for general examples (single resource, simple CRUD): AWS, Azure, GCP, OCI, OVH — each at `https://github.com/platform-engineering-labs/formae-plugin-<provider>`.

For specific advanced patterns, the canonical references are:

| Pattern | Canonical reference |
|---|---|
| Resolvable computed outputs (custom Read, output-port story) | [`formae-plugin-aws/schema/pkl/ses/emailidentity.pkl`](https://github.com/platform-engineering-labs/formae-plugin-aws/blob/main/schema/pkl/ses/emailidentity.pkl) |
| Polymorphic Target auth (abstract + discriminated subclasses) | [`formae-plugin-k8s/schema/pkl/main/k8s.pkl`](https://github.com/platform-engineering-labs/formae-plugin-k8s/blob/main/schema/pkl/main/k8s.pkl) |
| Cross-plugin Target Resolvables (Target Config field accepts upstream resolvable) | [`formae-plugin-grafana/schema/pkl/grafana.pkl`](https://github.com/platform-engineering-labs/formae-plugin-grafana/blob/main/schema/pkl/grafana.pkl) |
| Collection Resolvables (`Mapping<String, formae.Resolvable>`) | `formae-plugin-compose` — see the Target Config file with the Mapping field |
| Synthetic identifiers (`identifier = "Label"` for logical resources without natural server-side IDs) | `formae-plugin-atlas` — Migration resource schema |

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

**Target Config field serialization (G-6).** Plugin Go structs SHOULD declare camelCase json tags on `Config` fields (e.g., `` `json:"host"` ``). PKL fields should be single-name camelCase. Do NOT use the `hidden lowercase` / `fixed PascalCase = lowercase` pattern preserved in older plugins (k8s, Grafana) — that's a wart from Go structs lacking json tags, not a convention. Canonical reference for the clean pattern: the AWS plugin's Target Config classes.

**Target Config field immutability (G-8).** `@formae.ConfigFieldHint.createOnly` defaults to `true` (immutable), the **opposite** of `@formae.FieldHint` on resource fields (defaults to `false`, mutable). This is by-design: a Target's identity is typically defined by immutable environment fields (AWS account ID, region, server URL), so changing one means replacing the Target — and everything in it. Always declare `createOnly` **explicitly** on `ConfigFieldHint` annotations so intent is visible in PKL:

```pkl
@formae.ConfigFieldHint { createOnly = false }  // mutable
hidden host: String

@formae.ConfigFieldHint { createOnly = true }   // immutable (also the default)
hidden region: String
```

**MANDATORY: TDD for steps 4–8.** Each tutorial lesson includes integration tests (e.g., `TestCreate`, `TestRead`, `TestReadNotFound`, `TestUpdate`, `TestDelete`, `TestDeleteNotFound`, `TestList`). For EVERY CRUD operation you MUST follow this exact loop:

1. **Fetch the tutorial page** for that operation (e.g., `https://docs.formae.io/en/latest/plugin-sdk/tutorial/05-create/`)
2. **Write the integration test FIRST** — adapt the tutorial's test to the target provider. The test must compile but fail because the operation is not yet implemented.
3. **Run the test** — confirm it fails for the right reason (not implemented, not a compile error).
4. **Implement the operation** — write the minimum code to make the test pass.
5. **Run `make install && go test -tags=integration ./...`** — confirm the test passes.
6. **Move to the next operation** — do NOT skip ahead.

NEVER write implementation code before its corresponding test. NEVER skip writing tests. If you implement Create without first writing TestCreate, you are doing it wrong.

**Guided mode checkpoints** (skip all in autonomous mode):
- After step 1 (schema): present PKL resource definitions for review
- After step 2 (target): present target config approach for review
- After each CRUD operation (steps 4–8): present the failing test for review, then present the passing implementation for review
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

### 8. Installing the finished plugin on the agent

**Authoring vs. runtime distinction.** Schema work (`formae eval`, PKL editing) does NOT require the plugin to be installed on the agent — the PKL schema is evaluated locally. The plugin binary is only needed for `formae apply` / runtime operations that actually call cloud APIs.

**Where the plugin runs.** A resource plugin is a separate process that the formae agent discovers and spawns at startup. The binary must be present on the **agent machine** — the host (local or remote) where `formae agent start` runs. The agent's plugin directory is configured in `~/.config/formae/formae.conf.pkl` on that machine.

**Install requires root and possibly remote access.** Placing a binary in the agent's plugin directory typically requires `sudo` (or equivalent) on the agent host. If the agent runs remotely, you may need SSH access. If the agent runs in a container, you need to rebuild or extend the container image.

**The assistant guides, never installs.** Walk the user through the installation steps but do NOT:
- Run `sudo` commands to place binaries on the agent host
- SSH into a remote machine and install on the user's behalf
- Modify a production agent without the user's explicit confirmation for each step

**Docker-based agents.** If the agent runs in a Docker container, adding a plugin requires rebuilding or extending the image so the plugin binary is present inside it. The general approach is to add a `COPY` step for the plugin binary and a `RUN` step to register it, then redeploy the container. See [docs.formae.io](https://docs.formae.io) for the authoritative procedure.

> **Docs gap (as of 2026-06-17):** The page `https://docs.formae.io/en/operations/extending-the-agent-image/` is listed in the docs sidebar but returns 404. Until it is published, guide the user through the general Docker extension pattern and refer them to [docs.formae.io](https://docs.formae.io) for the latest authoritative steps. Do not fabricate specific Dockerfile snippets as authoritative — confirm with the user before any production change.

**Non-Docker agents.** Copy the built plugin binary to the agent host's plugin directory (as shown in `formae-plugin.pkl`'s install target or via `make install` if the agent machine is local), then restart the agent. The exact path is in `~/.config/formae/formae.conf.pkl` on the agent host.

**Verify the install.** After installation, run `formae plugin list` (or the equivalent agent health check) to confirm the agent has discovered and started the plugin before attempting `formae apply`.

### 9. Definition of Done

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
