# Lineage

Lineage is an open-source local distribution layer for packaging and sharing AI
agent workflows across Claude Code, Codex, and other coding agents.

It lets a prepared agent package travel as one environment: skills, workflows,
agents, policies, references, setup material, and provider entrypoints can be
bundled together, inspected, enabled, and launched from a receiver's machine.
The package stays local and readable; the receiver keeps using their own agent
tools.

The goal is simple:

```text
prepare once -> package safely -> share -> enable locally -> run with the user's own agent tools
```

Lineage does not try to replace the agent provider. It sits around local agent
commands and makes the surrounding environment easier to package, review, and
reproduce.

## What Lineage Is For

Use Lineage when a workflow is more than a prompt and you want to share the
working environment around it:

- Package Claude Code workflows, Codex workflows, agent skills, policies,
  references, and setup material together.
- Publish a reusable agent workflow to the Lineage registry or share it as a
  deterministic `.tgz` archive.
- Let receivers inspect the package contents and declared capabilities before
  enabling anything locally.
- Run the same packaged behavior through provider adapters instead of rebuilding
  it by hand for each project.

If you are looking for "how to share Claude workflows", "how to package AI
agent skills", or "how to distribute reusable agent workflows", Lineage is the
small local runtime and package format for that job.

## Install

```bash
curl -fsSL https://agenticlineage.vercel.app/install.sh | sh
```

Downloads the right prebuilt binary for your OS/architecture (macOS/Linux, amd64/arm64), verifies it against the release's published checksum, and installs it to `~/.lineage/bin`. No Go toolchain required. Windows: download `lineage-windows-amd64.exe` directly from the [latest release](https://github.com/agentic-lineage/lineage/releases/latest).

Go developers can also build from source:

```bash
go install github.com/agentic-lineage/lineage/cmd/lineage@latest
```

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

## How Lineage Fits Claude, Codex, and Other Agents

Lineage is provider-adjacent, not provider-owned. It prepares the local files a
provider already knows how to read, keeps provider-specific behavior behind
adapter boundaries, and can preview what it would materialize before writing.

- `lineage run claude --dry-run` previews Claude materialization.
- `lineage run codex --dry-run` previews Codex materialization.
- `lineage run auggie --dry-run` previews Auggie materialization.
- `lineage workflow run <workflow-name> <claude|codex|auggie> --dry-run` narrows the
  launch plan to one exported workflow.

The current adapters support Claude, Codex, and Auggie. The package shape is
deliberately plain so future adapters can use the same manifest, skills,
workflows, agents, policies, references, and setup material.

## Current Commands

```bash
lineage init user
lineage init workspace <name>

lineage package init <name>
lineage package validate <path>
lineage package export <path> [-o file.tgz]
lineage package import <file.tgz> [--as name]

lineage add <package-ref> [--yes]
lineage package publish <path>
lineage package pull <package-ref> [--as name]
lineage login
lineage logout
lineage whoami

lineage enable <package-path-or-id>
lineage disable <package-path-or-id>
lineage list
lineage inspect <package-path-or-id>
lineage run claude --dry-run
lineage run codex --dry-run
lineage run auggie --dry-run
lineage workflow run <workflow-name> <claude|codex|auggie> [--dry-run] [--yes]

lineage install-shims
lineage doctor
lineage version
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
  auggie:
    binary: /path/to/real/auggie
```

### Auggie setup and limitations

Auggie is installed and authenticated separately on the receiving machine. It
currently requires Node.js 22 or later:

```bash
npm install -g @augmentcode/auggie
auggie login
lineage run auggie --dry-run
```

The adapter finds `auggie` on `PATH`, or uses `providers.auggie.binary` when
configured. When approved, Lineage stages package skills in
`.augment/skills/`, updates its generated section in the project's `AGENTS.md`,
and launches Auggie with the supplied provider arguments.

Lineage does not read, package, or materialize Augment credentials, login or
session state, settings, user rules, MCP configuration, or other machine-local
files under `~/.augment`. Configure those features locally with Auggie. Auggie
is currently beta, so its own platform and terminal limitations still apply.

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

Install local shims when you want commands such as `claude`, `codex`, or
`auggie` to enter Lineage first:

```bash
lineage install-shims
```

Share a package as a file:

```bash
lineage package validate ./resume-workflow
lineage package export ./resume-workflow -o resume-workflow.tgz
```

On the receiving end:

```bash
lineage package import resume-workflow.tgz
lineage enable resume-workflow
```

Or publish it to the Lineage registry instead of passing a file around. No token to request first - `lineage login` authenticates you with your own GitHub account (the same device-flow approval `gh auth login` uses):

```bash
lineage login   # opens a code + a github.com link to approve once
lineage package publish ./resume-workflow
```

On the receiving end, the shortest path is `lineage add`. It fetches the package, shows what it contains, asks before enabling unless `--yes` is passed, and records it in the current project:

```bash
lineage add resume-workflow
```

For scripts or bootstrap prompts where the caller has already decided to install the package:

```bash
lineage add resume-workflow --yes
```

You can still pull and enable in two explicit steps if you want to inspect the imported copy yourself first. Pulling is an open read and does not require login:

```bash
lineage package pull resume-workflow
lineage enable resume-workflow
```

`lineage add` and `lineage package pull` accept `resume-workflow` for the latest version or `resume-workflow@0.2.0` for an exact version. Both verify the registry-reported digest against the package contents before keeping anything - see [docs/decisions/0012-v1-distribution-contract-and-receiver-activation.md](docs/decisions/0012-v1-distribution-contract-and-receiver-activation.md) for how the registry is structured.

The first publish of a package name claims it for your verified GitHub login. To ship an update, bump `version` in `lineage.yaml` and run `lineage package publish` again - the registry accepts it because you're still the recorded owner of that name; a different GitHub account would be rejected. Run `lineage whoami` any time to check which identity is currently active, or `lineage logout` to clear it. For non-interactive use (CI), set `LINEAGE_PUBLISH_TOKEN` to any GitHub-issued token with `read:user` access instead of running `lineage login`.

Published packages are browsable at [agenticlineage.vercel.app/packages](https://agenticlineage.vercel.app/packages). A package detail page includes the package version, digest, publisher, raw archive download, and a copy-paste bootstrap prompt for someone who has never installed Lineage before. The canonical bootstrap prompt lives in [docs/bootstrap-prompt.md](docs/bootstrap-prompt.md).

Useful day-to-day checks:

```bash
lineage list
lineage inspect resume-workflow
lineage doctor
lineage workflow run resume-review claude --dry-run
```

## Guides

- [What is a Lineage package?](docs/guides/lineage-package.md)
- [How to share Claude Code and Codex workflows](docs/guides/share-agent-workflows.md)
- [Architecture and trust model](docs/architecture.md)
- [Public docs and discoverability checklist](docs/discoverability.md)

## Safety Principles

- Packages should be inspectable before they are enabled.
- Secrets, credentials, provider login state, and private machine-local files should not be packaged.
- Setup actions should be explicit and permission-gated.
- Package behavior should be idempotent where possible.
- Provider-specific behavior should stay behind clear adapter boundaries.
- Declared capabilities are visible to receivers but are not a sandbox in this build.

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
Release and stable-branch rules are documented in
[docs/release-versioning.md](docs/release-versioning.md).
GitHub and website discoverability checks live in
[docs/discoverability.md](docs/discoverability.md).
When behavior affects install, publishing, receiver activation, setup prompts,
or safety wording, also check [docs/public-docs-sync.md](docs/public-docs-sync.md)
so the website, Wiki, package pages, and Discussions do not drift.

## License

MIT
