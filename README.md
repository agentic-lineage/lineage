# Lineage

Package and share Claude Code and Codex workflow environments.

A reusable agent workflow usually depends on more than one prompt or skill:
skills, workflow steps, agents, policies, references, setup files, declared
capabilities, and provider entrypoints all shape how it behaves. Lineage keeps
those pieces together as a local, inspectable package that another developer can
add to their own project and run with their own agent tools.

```bash
# author
lineage package validate ./resume-workflow
lineage login
lineage package publish ./resume-workflow

# receiver
lineage add resume-workflow
lineage run claude --dry-run
```

`lineage add` fetches the package, shows what it contains, asks before enabling
unless `--yes` is passed, and records it in the current project. `lineage run`
can preview the provider-specific files it would stage before anything is
written.

Use Lineage when you want to:

- Package Claude Code workflows, Codex workflows, skills, agents, policies,
  references, setup material, and adapter entrypoints together.
- Publish a workflow package to the Lineage registry or share it as a
  deterministic `.tgz` archive.
- Let receivers inspect package contents and declared capabilities before
  enabling the workflow locally.
- Run the packaged workflow through Claude Code or Codex without rebuilding the
  environment by hand in each project.

Lineage is provider-adjacent, not provider-owned. It prepares local files for
the agent provider the receiver already uses; it does not replace Claude Code,
Codex, or the user's agent account.

## Install

```bash
curl -fsSL https://agenticlineage.vercel.app/install.sh | sh
```

Downloads the right prebuilt binary for your OS/architecture (macOS/Linux,
amd64/arm64), verifies it against the release's published checksum, and installs
it to `~/.lineage/bin`. No Go toolchain required. Windows: download
`lineage-windows-amd64.exe` directly from the
[latest release](https://github.com/agentic-lineage/lineage/releases/latest).

Go developers can also build from source:

```bash
go install github.com/agentic-lineage/lineage/cmd/lineage@latest
```

## Quick Start

Create a package:

```bash
lineage package init resume-workflow
```

Add the workflow's skills, workflow steps, agents, policies, references, and
setup material to the generated folders. Keep secrets, provider login state,
private machine paths, and local credentials out of the package.

Validate and preview it locally:

```bash
lineage package validate ./resume-workflow
lineage enable ./resume-workflow
lineage run claude --dry-run
lineage run codex --dry-run
```

Publish it to the Lineage registry:

```bash
lineage login
lineage package publish ./resume-workflow
```

The first publish of a package name claims it for your verified GitHub login.
To ship an update, bump `version` in `lineage.yaml` and run
`lineage package publish` again. Run `lineage whoami` to check the active
identity, or `lineage logout` to clear it. For non-interactive use, set
`LINEAGE_PUBLISH_TOKEN` to a GitHub-issued token with `read:user` access instead
of running `lineage login`; publishing uses GitHub only to identify the
publisher, so repository write scopes are not required.

On the receiving end:

```bash
lineage add resume-workflow
lineage inspect resume-workflow
lineage run claude --dry-run
```

For scripts or bootstrap prompts where the caller has already decided to install
the package:

```bash
lineage add resume-workflow --yes
```

`lineage add` and `lineage package pull` accept `resume-workflow` for the latest
version or `resume-workflow@0.2.0` for an exact version. Both verify the
registry-reported digest against the package contents before keeping anything.
See
[docs/decisions/0012-v1-distribution-contract-and-receiver-activation.md](docs/decisions/0012-v1-distribution-contract-and-receiver-activation.md)
for the registry structure.

That same exact-ref form is also how you move a project back to a previous
version; there is no separate `rollback` command yet
([#122](https://github.com/agentic-lineage/lineage/issues/122)), because
pinning already covers it. If that version is not already present locally,
`lineage add` pulls it and then enables it for the current project.

There is no way to remove a published version yet. The planned V1 behavior
([#123](https://github.com/agentic-lineage/lineage/issues/123)) is to yank a
version: hide it from the package directory and from bare-name latest
resolution behind a tombstone, while leaving it addressable by exact version
for anyone who already depends on it.

Published packages are browsable at
[agenticlineage.vercel.app/packages](https://agenticlineage.vercel.app/packages).
A package detail page includes the package version, digest, publisher, raw
archive download, and a copy-paste bootstrap prompt for someone who has never
installed Lineage before. The canonical bootstrap prompt lives in
[docs/bootstrap-prompt.md](docs/bootstrap-prompt.md).

The production registry is served by the Lineage website API and currently
stores package metadata/aggregate metrics in Supabase Postgres with immutable
archives in a private Supabase Storage bucket. Publisher identity is still
verified through GitHub (`lineage login` or `LINEAGE_PUBLISH_TOKEN` with
`read:user`), and the original GitHub Releases registry remains the migration
source and rollback path while the Supabase-backed registry is exercised in
production.

## Sharing A `.tgz` Archive

You can share a package without the registry:

```bash
lineage package validate ./resume-workflow
lineage package export ./resume-workflow -o resume-workflow.tgz
```

On the receiving end:

```bash
lineage package import resume-workflow.tgz
lineage enable resume-workflow
lineage run codex --dry-run
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

These folders are intentionally plain. A receiver should be able to open the
package and see what it contains before enabling it.

## How Lineage Fits Claude, Codex, And Other Agents

Lineage keeps provider-specific behavior behind adapter boundaries and previews
what it would materialize before writing.

- `lineage run claude --dry-run` previews Claude materialization.
- `lineage run codex --dry-run` previews Codex materialization.
- `lineage workflow run <workflow-name> <claude|codex> --dry-run` narrows the
  launch plan to one exported workflow.

The current adapters focus on Claude and Codex. The package shape stays plain so
future adapters can use the same manifest, skills, workflows, agents, policies,
references, and setup material.

For the contributor-facing work to compile an existing agent workspace into
those portable artifacts, see
[Compiling Existing Workspaces](docs/guides/compiling-existing-workspaces.md).

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

The first time `lineage run` would stage files for a provider, it shows what it
is about to create or change and asks for confirmation. Pass `--yes`/`-y` to
skip the prompt in scripts. `--dry-run` never writes anything.

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
lineage workflow run <workflow-name> <claude|codex> [--dry-run] [--yes]

lineage install-shims
lineage doctor
lineage version
```

Useful day-to-day checks:

```bash
lineage list
lineage inspect resume-workflow
lineage doctor
lineage workflow run resume-review claude --dry-run
```

Install local shims when you want commands such as `claude` or `codex` to enter
Lineage first:

```bash
lineage install-shims
```

You can also pull and enable in two explicit steps if you want to inspect the
imported copy yourself first. Pulling is an open read and does not require
login:

```bash
lineage package pull resume-workflow
lineage enable resume-workflow
```

## Guides

- [What is a Lineage package?](docs/guides/lineage-package.md)
- [How to share Claude Code and Codex workflows](docs/guides/share-agent-workflows.md)
- [Compiling an existing agent workspace](docs/guides/compiling-existing-workspaces.md)
- [Architecture and trust model](docs/architecture.md)
- [Public docs and discoverability checklist](docs/discoverability.md)

## Safety

Packages are meant to be inspected before they are enabled, and generated
provider files can be previewed with `--dry-run` before they are written. The
canonical safety model, including current checks, warnings, and non-goals, lives
in [docs/safety.md](docs/safety.md).

## Development

```bash
go test ./...
go run ./cmd/lineage --help
```

The source follows the standard Go layout:

- `cmd/lineage` contains the CLI entrypoint.
- `internal/` contains runtime, package, config, provider, and shim code.
- `.agents/skills` contains repository-native guardrail skills for consistent
  agent-assisted development.

Important project decisions are recorded in
[docs/decisions](docs/decisions/README.md). Release and stable-branch rules are
documented in [docs/release-versioning.md](docs/release-versioning.md). GitHub
and website discoverability checks live in
[docs/discoverability.md](docs/discoverability.md). When behavior affects
install, publishing, receiver activation, setup prompts, or safety wording, also
check [docs/public-docs-sync.md](docs/public-docs-sync.md) so the website, Wiki,
package pages, and Discussions do not drift.

## License

MIT
