# Architecture

Lineage is a local runtime for distributing agent environment packages.

## Core Ideas

- A package is a folder with a `lineage.yaml` manifest and optional native assets such as skills, workflows, agents, policies, references, and provider adapters.
- A project enables packages through `.lineage/config.yaml`.
- The runtime resolves enabled packages, builds a launch plan, and starts the selected provider through an explicit adapter boundary.
- Provider shims let a normal command such as `claude` or `codex` enter the Lineage runtime first, then hand off to the real provider binary.

## Package Shape

```text
package/
├── lineage.yaml
├── skills/
├── workflows/
├── agents/
├── policies/
├── references/
└── adapters/
```

The receiver should be able to inspect these contents before enabling a package.

## Boundaries

- `cmd/lineage`: CLI entrypoint.
- `internal/cli`: command routing and user-facing command behavior.
- `internal/config`: project and user configuration.
- `internal/packages`: package manifest, discovery, and reference resolution.
- `internal/runtime`: provider-neutral launch planning.
- `internal/provider`: provider command resolution and execution.
- `internal/shim`: local command shims.

## Safety Model

Packages are local files, but they are still untrusted input. Lineage should validate manifests, normalize paths, avoid secret capture, and require explicit permission before any setup flow creates or changes files.

## Decision Records

Architecture decisions are tracked in the [decision log](decisions/README.md).
