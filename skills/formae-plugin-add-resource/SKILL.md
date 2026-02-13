---
name: formae-plugin-add-resource
description: "Use when the user wants to add support for a new resource type to an existing formae plugin"
---

# Add a Resource Type to an Existing Plugin

Add support for a new resource type to an existing formae resource plugin, following the plugin SDK tutorial and using TDD.

## MANDATORY RULE: Install Before Testing

After ANY code changes to the plugin, you MUST run `make install` before running tests. Integration tests and conformance tests run against the INSTALLED plugin binary, not the source code. Skipping this step means you're testing stale code.

## Workflow

### 1. Locate the plugin

Ask the user where their plugin lives. Check that the directory exists and contains a valid plugin project (look for `formae-plugin.pkl`, `Makefile`, and the plugin Go source). Run `/init` so you have full context on the codebase.

### 2. Choose the resource type

Ask the user which resource type they want to add. Then research the provider's API for that resource thoroughly:

- Resource model: required vs optional fields, immutable fields (createOnly), read-only fields
- CRUD operations: endpoints, request/response shapes, error codes
- Async operations: does the API return immediately or require polling?
- Value normalization: does the API normalize values? (e.g., case changes, default values injected server-side)
- Relationships: does this resource reference or depend on other resources?

Look at how Terraform or Pulumi model the equivalent resource for schema design reference.

Present your research findings and a proposed resource schema to the user for approval before proceeding.

### 3. Follow the tutorial (steps 2–10)

Fetch the plugin SDK tutorial from `https://docs.formae.io/en/latest/plugin-sdk/tutorial/` and follow steps 2–10, adapting each lesson to the new resource type. Study the existing resource implementations in the plugin for patterns and conventions to follow.

Reference prior art plugins for real-world patterns:
- AWS: `https://github.com/platform-engineering-labs/formae-plugin-aws`
- Azure: `https://github.com/platform-engineering-labs/formae-plugin-azure`
- GCP: `https://github.com/platform-engineering-labs/formae-plugin-gcp`
- OCI: `https://github.com/platform-engineering-labs/formae-plugin-oci`
- OVH: `https://github.com/platform-engineering-labs/formae-plugin-ovh`

Steps to follow:

1. **Schema** — add the new PKL resource type with annotations (`@ResourceType`, `@CreateOnly`, `@ReadOnly`)
2. **Plugin registration** — register the new resource type in the plugin's type switch/router
3. **Create** — implement resource creation
4. **Read** — implement resource reading (handle NotFound correctly)
5. **Update** — implement resource updates
6. **Delete** — implement resource deletion (handle NotFound as success)
7. **List** — implement resource discovery
8. **Error handling** — use `OperationErrorCode` patterns from the SDK

**MANDATORY: TDD for steps 3–7.** Each tutorial lesson includes integration tests (e.g., `TestCreate`, `TestRead`, `TestReadNotFound`, `TestUpdate`, `TestDelete`, `TestDeleteNotFound`, `TestList`). For EVERY CRUD operation you MUST follow this exact loop:

1. **Fetch the tutorial page** for that operation (e.g., `https://docs.formae.io/en/latest/plugin-sdk/tutorial/05-create/`)
2. **Write the integration test FIRST** — adapt the tutorial's test to the new resource type. Follow the conventions used by existing tests in the plugin.
3. **Run the test** — confirm it fails for the right reason (not implemented, not a compile error).
4. **Implement the operation** — write the minimum code to make the test pass.
5. **Run `make install && go test -tags=integration ./...`** — confirm the test passes.
6. **Move to the next operation** — do NOT skip ahead.

NEVER write implementation code before its corresponding test. NEVER skip writing tests.

### 4. Conformance tests

Run `make install && make conformance-test`.

Fix any failures and re-run. All CRUD and discovery tests must pass before continuing.

### 5. Local end-to-end test

Ask the user if they want to do a manual end-to-end test. If yes:

1. Write a simple forma file using the new resource type
2. `formae apply --mode reconcile --simulate` to verify the plan
3. `formae apply --mode reconcile` to create real resources
4. `formae inventory` to verify resources are managed
5. `formae destroy` to clean up

### 6. Documentation

Update the plugin's `README.md`:
- Add the new resource type to the supported resources table
- Add example usage if the resource has non-obvious configuration

### 7. Definition of Done

Before reporting completion, verify each item:

- [ ] PKL schema defined with correct annotations
- [ ] Full CRUD implemented and integration tests passing
- [ ] Discovery (List) implemented and tested
- [ ] Conformance tests pass (`make install && make conformance-test`)
- [ ] README updated with the new resource type

## Important

- After ANY code changes, ALWAYS run `make install` before running tests — tests run against the installed binary
- Follow the existing patterns in the plugin — match naming conventions, error handling style, and test structure
- NEVER use `pkl eval` to evaluate forma files — ALWAYS use `formae eval --output-consumer machine`
- NEVER skip integration tests — they verify the CRUD lifecycle against the real provider API
- NEVER ask the user to run tests — run them yourself and report results
