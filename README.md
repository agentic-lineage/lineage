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
lineage package validate <path>
lineage package export <path> [-o file.tgz]
lineage package import <file.tgz> [--as name]
lineage enable <package-path-or-id>
lineage run claude --dry-run
lineage run codex --dry-run
lineage install-shims
```

The first time `lineage run` would actually stage files for a provider, it shows what it's about to create or change and asks for confirmation; pass `--yes`/`-y` to skip the prompt in scripts. `--dry-run` never writes anything.

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

Share a package with someone else:

```bash
lineage package validate ./resume-workflow
lineage package export ./resume-workflow -o resume-workflow.tgz
```

On the receiving end:

```bash
lineage package import resume-workflow.tgz
lineage enable resume-workflow
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
