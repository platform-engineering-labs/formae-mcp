// Copyright 2026 Platform Engineering Labs Inc.
// SPDX-License-Identifier: FSL-1.1-ALv2

package tools

type CodebaseContextInput struct {
	Profile          string   `json:"profile,omitempty" jsonschema:"Profile to resolve for this call. Use an explicit profile to keep concurrent sessions independent."`
	Mode             string   `json:"mode,omitempty" jsonschema:"Optional explicit local source context: none or codebase. Codebase requires binding_id; none cannot include binding_id. Omit to discover a registered project from working_directory or return candidates."`
	BindingID        string   `json:"binding_id,omitempty" jsonschema:"Registered project binding selected for this call, returned by get_codebase_context or list_codebases."`
	WorkingDirectory string   `json:"working_directory,omitempty" jsonschema:"Absolute harness workspace directory for project discovery. Supply the actual caller workspace; the MCP process directory is not used."`
	Stacks           []string `json:"stacks,omitempty" jsonschema:"Stack labels to check against the selected project scope. This discovery check does not replace validation of the complete evaluated apply scope."`
}

type RegisterCodebaseInput struct {
	Profile string   `json:"profile,omitempty" jsonschema:"Profile whose resolved installation this project will track for this call."`
	Path    string   `json:"path" jsonschema:"required,Absolute path to an existing project directory containing PklProject. Register after the user opts into maintaining this codebase."`
	Stacks  []string `json:"stacks,omitempty" jsonschema:"Optional exact stack labels maintained by this project. Omit for unrestricted installation scope. Re-registering this path and installation updates its scope."`
}

type UnregisterCodebaseInput struct {
	Profile   string `json:"profile,omitempty" jsonschema:"Profile for the installation owning this binding."`
	BindingID string `json:"binding_id" jsonschema:"required,Exact local binding ID to remove. The project files remain available."`
}

type DriftPreferenceInput struct {
	Profile string `json:"profile,omitempty" jsonschema:"Profile resolving the installation for this local preference."`
	Mode    string `json:"mode" jsonschema:"required,prompt or auto_absorb_external. Set only after the user explicitly chooses. Automatic acceptance covers proven nonconflicting external changes only; patches and uncertain or mixed history still require a decision."`
}

const DriftPreferenceDescription = `Save the user's local drift preference for the resolved installation, across sessions and codebases. Only call after an explicit user choice. Only while the preference is unset and readable, offer after the user explicitly keeps a change proven external-only by the original observation: ask each time (prompt), or automatically keep future nonconflicting external changes, including external deletions (auto_absorb_external). Ask once during the operation; do not offer after revert alone or keeping a patch. Keeping the current change is not consent to a future policy. A saved explicit prompt suppresses future preference offers. No answer leaves the default unchanged and does not block the current apply. Show accepted deletions as removals from desired state in the final preview and message. Patches always require a decision. The preference never authorizes force, conflicting changes, guessing unknown provenance, or skipping ordinary cloud-write confirmation. Read the effective preference through get_codebase_context; apply rejections refresh it. The user can change it at any time.`

const CodebaseContextDescription = `Resolve the source selection from the local registry before IaC file search or reads for infrastructure authoring/apply. Source-only inspection of an explicitly supplied file does not require this tool. Reuse that selection for the same installation/workspace until the user changes it or context validation fails; do not rediscover a project for every operation. A none selection is definitive: use fresh prepare_authoring source, never search disk for a missing project. An explicit binding or none selection wins; otherwise use the supplied harness workspace to find a registered project, or return candidates for selection. Missing projects and corrupt registration remain explicit errors. Carry a selected codebase as context {mode:codebase,binding_id} on apply and policy planner calls. For none, use prepare_authoring to obtain disposable source and its per-call context; connected capabilities are checked by the consumers. Selection alone does not prove agent support. Never replace this tool with recursive home/workspace scans, auto-register found folders, or interpret stale local Pkl as pending formae intent. The result also includes the local installation drift_preference (prompt by default). Rejections refresh that preference, so cached source selection does not authorize stale automatic acceptance. There is no global active project.`

const ListCodebasesDescription = `List registered local IaC projects for the resolved installation, including missing project directories and their availability. Registration is independent of profile names. Use the returned binding ID to select a project for a call.`

const RegisterCodebaseDescription = `Register an existing local IaC project for the resolved installation after the user opts into maintaining it. The directory must already contain PklProject. This records the canonical project path and optional stack scope; initialization and codebase catch-up are separate steps. Multiple projects can track one installation.`

const UnregisterCodebaseDescription = `Remove a local project's registration for the resolved installation. This removes the registry binding only; project files and cloud resources are retained. It also works when the registered directory is missing.`
