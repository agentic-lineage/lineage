# Lineage

Lineage is a local distribution layer for agent environments.

It lets a prepared agent package travel as one environment: skills, workflows, agents, policies, references, setup material, and provider entrypoints can be bundled together, inspected, enabled, and launched from a receiver's machine.

The goal is simple:

```text
prepare once -> package safely -> share -> enable locally -> run with the user's own agent tools
```

Lineage does not try to replace the agent provider. It sits around local agent commands and makes the surrounding environment easier to package, review, and reproduce.

## What Is In A Package?

A Lineage package is a normal folder with a `lineage.yaml` manifest:

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

These folders are intentionally plain. A receiver should be able to open the package and see what it contains before enabling it.

## Current Commands

```bash
lineage init user
lineage init workspace <name>
lineage package init <name>
lineage enable <package-path-or-id>
lineage run claude --dry-run
lineage run codex --dry-run
lineage install-shims
```

Project configuration lives at `.lineage/config.yaml`.

```yaml
workspace: ""
enabled_packages:
  - ./my-agent-package
provider_preferences: {}
providers:
  claude:
    binary: /path/to/real/claude
  codex:
    binary: /path/to/real/codex
```

## Quick Start

Create a package:

```bash
lineage package init resume-workflow
```

Enable it inside a project:

```bash
lineage enable ./resume-workflow
```

Preview the launch plan:

```bash
lineage run claude --dry-run
```

Install local shims when you want commands such as `claude` or `codex` to enter Lineage first:

```bash
lineage install-shims
```

## Safety Principles

- Packages should be inspectable before they are enabled.
- Secrets, credentials, provider login state, and private machine-local files should not be packaged.
- Setup actions should be explicit and permission-gated.
- Package behavior should be idempotent where possible.
- Provider-specific behavior should stay behind clear adapter boundaries.

## Development

```bash
go test ./...
go run ./cmd/lineage --help
```

The source follows the standard Go layout:

- `cmd/lineage` contains the CLI entrypoint.
- `internal/` contains runtime, package, config, provider, and shim code.
- `.agents/skills` contains repository-native guardrail skills for consistent agent-assisted development.

Important project decisions are recorded in [docs/decisions](docs/decisions/README.md).

## License

MIT
