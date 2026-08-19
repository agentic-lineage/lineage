# Architecture

Lineage is a local runtime for distributing agent environment packages.

## Core Ideas

- A package is a folder with a `lineage.yaml` manifest and optional native assets such as skills, workflows, agents, policies, references, and provider adapters. The manifest carries a `schema` version and a content digest, and can declare exports and capabilities explicitly.
- A project enables packages through `.lineage/config.yaml`.
- Enabling a package doesn't just plan a launch — running a provider materializes it: skills are staged into that provider's own discovery directory (e.g. `.claude/skills/`), and a generated section is written into its context file (e.g. `CLAUDE.md`), tracked per-provider so re-running stays idempotent. This is gated behind an explicit confirmation the first time it would actually change anything.
- A package can be exported to a deterministic `.tgz` archive and imported on another machine; import re-validates the extracted content exactly as `lineage package validate` would, rather than trusting the archive because it parsed.
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
- `internal/packages`: package manifest, discovery, reference resolution, validation, and export/import.
- `internal/materialize`: stages an enabled package's skills and generated context into a provider's own directories, and tracks that state per provider so re-running stays idempotent.
- `internal/runtime`: provider-neutral launch planning.
- `internal/provider`: the provider registry, command resolution, and execution.
- `internal/shim`: local command shims.

## Safety Model

Packages are local files, but they are still untrusted input. Manifests are validated, package-controlled paths (entrypoints, archive entries) are normalized through a traversal guard, package content is scanned against a documented secret allow/denylist, and materialization requires explicit permission before it creates or changes files. An imported archive gets no special trust for having been exported by Lineage — import re-runs the full validation pass against the extracted content before anything is kept.

## Decision Records

Architecture decisions are tracked in the [decision log](decisions/README.md).
