---
name: formae-import
description: "Use when the user wants to bring unmanaged/discovered resources under formae management, import resources into their IaC codebase, or absorb cloud resources into existing forma files"
---

# Import Unmanaged Resources

Bring discovered (unmanaged) resources under formae management by incorporating them into an existing IaC codebase.

## Workflow

### 1. Identify the IaC codebase

The target codebase is typically the current working directory (the git repo Claude was started from). Confirm with the user:
- State the current working directory and ask if that's the right codebase
- If not, ask them to specify the directory containing their forma/PKL files

### 2. Discover unmanaged resources

First call `get_agent_stats` to get an overview of unmanaged resource counts by provider. Then use targeted `list_resources` queries with specific type filters (e.g., `managed:false type:AWS::S3::Bucket`) to drill down. **Never** call `list_resources` with just `managed:false` — on real accounts this returns too much data.

Present a summary of what's available by type and count.

### 3. User selects resources

Ask the user which resource(s) they want to import. They may select one, several, or all of a given type.

### 4. Extract as PKL

Call `extract_resources` with a query that matches the selected resources. This returns the PKL representation of those resources as they exist in the cloud right now.

### 5. Read the existing IaC codebase

Read the user's existing forma files to understand:
- **Module composition**: which file is the main forma file (it `amends "@formae/forma.pkl"` and has the `forma {}` block), and how helper modules are imported and spread into it
- **File organization**: how resources are grouped into files
- **Naming conventions**: variable names, labels, prefixes
- **Abstractions**: shared variables (e.g., `vars.pkl`), parametrized modules
- **Stack and target definitions**: where they're defined and how they're referenced
- **Import patterns**: `@formae/`, `@aws/`, local imports

### 6. Confirm stack assignment

Before writing any code, confirm with the user which stack the imported resources should belong to. Suggest the most appropriate stack based on:
- The resource type and its purpose
- Existing stack patterns in the codebase

**CHECKPOINT: Do not proceed until the user confirms the stack.**

### 7. Target assignment

Use the target from the extracted resource. It's already known from discovery since the agent discovers per-target.

### 8. Incorporate idiomatically

Merge the extracted PKL into the existing codebase following its conventions:

**Critical: Resource labels must match exactly.** Formae identifies resources by the triplet (stack, type, label). When importing, you MUST use the exact same label as the unmanaged resource has in the agent. If you change the label, formae will see it as a brand new resource to create instead of an existing one to bring under management. Copy the label from the extracted PKL verbatim — do not rename, shorten, or prettify it.

**Critical: PKL module composition.** New resources must be part of the module tree rooted at the main forma file. There are two approaches:
- **Add to an existing file** if the resource fits naturally (e.g., an S3 bucket alongside other storage resources)
- **Create a new helper module** if a separate file is more idiomatic — but this file must be `import`ed by the main forma file and its resources spread into the `forma {}` block (e.g., `...importedModule.resources`)

A standalone PKL file that defines its own stack and target will **not work** with reconcile mode — it would be treated as a separate stack declaration and cause existing resources to be destroyed.

Additional guidelines:
- Use existing variable patterns (e.g., if there's a `vars.pkl` with shared config, reference it)
- Follow the existing naming style for variable names and code structure, but **never change resource labels**
- Reuse existing stack and target definitions rather than creating new ones
- Add any necessary imports

### 9. Verify the import is side-effect free

Run `apply_forma` with `mode: reconcile`, `simulate: true` on the **main forma file** (not the helper module).

Tell the user you're checking that the import won't cause any unintended changes — only the expected "bring under management" operations for the newly added resources.

Check the simulation result via `get_command_status`. The stopping criteria are strict:

- **Expected**: the simulation shows "bring under management" updates for the imported resources. This means formae matched each resource in the PKL to an existing unmanaged resource by (stack, type, label) and will adopt it.
- **NOT acceptable**: if the simulation shows a **create** operation, it means the label/type/stack triplet in your PKL does not match any existing unmanaged resource. The most common cause is that you changed the resource label. Fix the PKL to use the exact label from the extracted output and re-simulate.
- **NOT acceptable**: if the simulation shows **deletes** of existing resources, it means the module composition is wrong (e.g., standalone file instead of being wired into the main forma's import tree). Fix and re-simulate.
- **NOT acceptable**: property changes to existing managed resources — this means the import modified something it shouldn't have.

Loop until the simulation shows only "bring under management" for the imported resources and nothing else, or ask the user for help if stuck.

### 10. Apply

Once the simulation looks correct:
- Present the simulation results to the user
- **Ask for explicit confirmation** before proceeding
- Call `apply_forma` with `mode: reconcile`, `simulate: false`
- Monitor with `get_command_status` and report the result

## Important

- NEVER skip the simulation step
- NEVER apply without user confirmation
- NEVER use patch mode for imports — patch is for emergency changes only and creates drift
- NEVER change resource labels — they must match the unmanaged resource exactly for formae to recognize it as a "bring under management" operation
- A simulation showing **create** means the import is wrong — the label doesn't match. Fix it.
- A simulation showing **delete** means the module composition is wrong. Fix it.
- The only acceptable simulation result is "bring under management" for the imported resources with no other changes
