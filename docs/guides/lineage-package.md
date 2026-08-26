# What Is A Lineage Package?

A Lineage package is a local folder or deterministic archive that carries an AI
agent workflow's surrounding environment: skills, workflows, agents, policies,
references, setup material, and provider adapter entrypoints. It is meant to be
inspected before it is enabled, then run through the receiver's own Claude Code,
Codex, or other supported local agent tools.

The package root contains a `lineage.yaml` manifest:

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

The manifest names the package, records its schema version, declares exported
skills and workflows, and describes provider entrypoints and capabilities. The
filesystem stays plain so a receiver can review the contents without a special
viewer.

## Why Package A Workflow Instead Of Sharing A Prompt?

A prompt is usually only one part of a working agent environment. Real workflows
also depend on task-specific skills, reference files, setup steps, provider
instructions, and safety expectations. Lineage keeps those pieces together so a
workflow can be reviewed, moved, and reproduced without asking the receiver to
rebuild the same folder structure by hand.

Lineage does not replace the agent provider. It prepares local files for the
provider the receiver already uses, then hands off to that provider through an
adapter boundary.

## Common Package Commands

Create and validate a package:

```bash
lineage package init resume-workflow
lineage package validate ./resume-workflow
```

Export a package archive:

```bash
lineage package export ./resume-workflow -o resume-workflow.tgz
```

Publish and receive through the registry:

```bash
lineage login
lineage package publish ./resume-workflow
lineage add resume-workflow
```

Preview provider materialization before anything is written:

```bash
lineage run claude --dry-run
lineage run codex --dry-run
lineage run auggie --dry-run
lineage workflow run resume-review claude --dry-run
```

## Safety Model

Treat every package as untrusted input until it has been inspected. Lineage
validates manifests, checks package-controlled paths for traversal, scans for
common secret-shaped files and content, and asks for confirmation before first
materialization writes provider files.

Declared capabilities are visible metadata, not a sandbox. They help receivers
understand what a package says it needs before enabling it.

See [docs/safety.md](../safety.md) for the full model, including secret-scanning
limits and what's not implemented yet.

Related docs:

- [Safety model](../safety.md)
- [Architecture](../architecture.md)
- [Bootstrap prompt](../bootstrap-prompt.md)
- [Distribution contract](../decisions/0012-v1-distribution-contract-and-receiver-activation.md)
