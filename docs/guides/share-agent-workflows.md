# How To Share Claude Code And Codex Workflows

Lineage shares Claude Code and Codex workflows by packaging the local files
around the workflow, publishing or exporting that package, and letting the
receiver inspect and enable it in their own project. The receiver keeps using
their own agent tools; Lineage supplies the portable workflow environment.

## Author Flow

Create a package for the workflow:

```bash
lineage package init resume-workflow
```

Add the workflow's skills, workflow steps, references, policies, and setup
material to the package folders. Keep secrets, provider login state, private
machine paths, and local credentials out of the package.

Validate and preview before sharing:

```bash
lineage package validate ./resume-workflow
lineage enable ./resume-workflow
lineage run claude --dry-run
lineage run codex --dry-run
```

Publish through the Lineage registry:

```bash
lineage login
lineage package publish ./resume-workflow
```

The registry verifies your GitHub login, claims the package name for that
verified publisher on first publish, and stores immutable package archives
behind the Lineage website API. To publish a new version, bump `version` in
`lineage.yaml` and publish again; republishing the same `name@version` with a
different digest is rejected.

Or share a deterministic archive:

```bash
lineage package export ./resume-workflow -o resume-workflow.tgz
```

## Receiver Flow

For a published package, use `lineage add`:

```bash
lineage add resume-workflow
```

The package ref can be a bare name, which resolves to the latest registry
version, or an exact ref such as `resume-workflow@0.2.0`. Pull/add verifies the
registry digest against the downloaded archive before keeping it locally.

For a local archive, import and enable explicitly:

```bash
lineage package import resume-workflow.tgz
lineage enable resume-workflow
```

Preview the provider-specific plan before writing files:

```bash
lineage run claude --dry-run
lineage run codex --dry-run
```

Run one exported workflow when the package contains several:

```bash
lineage workflow run resume-review claude --dry-run
lineage workflow run resume-review codex --dry-run
```

## What Receivers Can Inspect

Before enabling, receivers can review:

- `lineage.yaml` manifest metadata, exports, entrypoints, and capabilities.
- `skills/`, `workflows/`, `agents/`, `policies/`, and `references/` content.
- Validation output, including portability and safety findings.
- Dry-run materialization output for Claude Code or Codex.

Lineage's goal is to make reusable agent workflows portable without hiding what
will be staged locally.

Related docs:

- [What is a Lineage package?](lineage-package.md)
- [Architecture](../architecture.md)
- [Public docs sync checklist](../public-docs-sync.md)
