# Safety Model

This is the canonical explanation of what Lineage checks, what it blocks, what it only warns about, and what remains the package author's, receiver's, or provider's own responsibility. It reflects current behavior only — it does not promise anything Lineage doesn't actually do yet. See [docs/architecture.md](architecture.md) for the shorter, system-level version this page expands on, and [docs/decisions/](decisions/README.md) for the reasoning behind each decision referenced below.

## Why This Page Exists

A Lineage package is a normal folder: skills, workflows, agents, policies, references, and setup material that travel together and get materialized into a provider's own directories. Nothing about that format is executable on its own, but its *content* can still instruct an agent to do something a receiver wouldn't have chosen — read files outside the workspace, reach an unexpected network endpoint, or embed a credential that gets redistributed by accident. Lineage's job is to make a package inspectable and its risks visible before any of that content reaches a running agent, not to promise a sandbox it doesn't have.

## The Package Lifecycle

A package passes through the same checks whether it was authored locally, imported from an archive, or pulled from the registry — later stages never trust an earlier stage's judgment call, they redo the check themselves:

```
author → validate → export/publish → import/pull → inspect → enable → materialize → run
```

- **Author**: `lineage package init` scaffolds the standard layout. Nothing is checked yet.
- **Validate**: `lineage package validate <path>` runs every check below against the package as it sits on disk, without enabling or writing anything.
- **Export/Publish**: `lineage package export`/`lineage package publish` refuse to run at all if `Validate` finds a blocking problem — a package that fails validation cannot leave the machine that authored it.
- **Import/Pull**: `lineage package import <archive>`/`lineage package pull <ref>` treat the incoming content as fully untrusted regardless of source (ADR 0011) — a previously-valid export that was edited, corrupted, or substituted in transit gets caught here, not assumed safe because it parsed.
- **Inspect**: `lineage inspect <ref>` shows a package's declared and discovered contents (manifest, skills/workflows/agents/policies, digest, declared capabilities) so a receiver can look before enabling.
- **Enable**: `lineage enable <ref>` adds a package to the current project's `.lineage/config.yaml`. If the package declares setup (files/directories its workflow expects), the receiver sees exactly what would be created and must approve it first.
- **Materialize**: `lineage run <provider>`/`lineage workflow run` stage the package's skills into the provider's own directory and write its generated context file — gated behind an explicit confirmation the first time a given package set would actually change anything on disk, and idempotent/reversible on every re-run (ADR 0008).

## What `lineage package validate` Checks

`internal/packages.Validate` (also run implicitly by `export`, `import`, and `publish`) checks, and reports every problem it finds rather than stopping at the first one:

- **Manifest schema**: `lineage.yaml` must declare a `schema` this build understands (ADR 0005) and load cleanly.
- **Export authority**: if `exports.agents`/`exports.workflows` names something, it must exist on disk — a declared-but-missing export is a blocking error, not a warning.
- **Entrypoint path safety**: `entrypoints.claude`/`entrypoints.codex` are checked with `SafeJoin`, a traversal guard scoped specifically to package-controlled input (ADR 0010) — this never applies to a path you type yourself at the CLI.
- **Secret scan**: see [Secret Scanning](#secret-scanning-what-it-catches-and-what-it-doesnt) below.
- **Content digest**: a `sha256` digest over the manifest and every content directory, computed in deterministic order, so `name@version` pairs with "and this is exactly what that resolved to" (ADR 0005).

A package with any blocking error fails validation and refuses to export/publish. Informational notes (e.g. a required skill this package doesn't itself provide, which may come from another package at enable time) don't block.

## What `lineage inspect` Shows

`lineage inspect <ref>` resolves a package (project path, user id, or workspace id) and shows its manifest, discovered skills/workflows/agents/policies/references, content digest, and declared capabilities — without enabling it or writing anything. This is the point in the lifecycle meant for a receiver to actually read what they're about to enable, before they type `lineage enable`.

## What `publish`, `pull`, And `import` Protect

- **Publish**: requires a logged-in identity (`lineage login`, or `LINEAGE_PUBLISH_TOKEN` for non-interactive use). The first publish of a name claims it; a later publish of the same name needs the same identity, so one publisher can't overwrite another's package.
- **Pull**: an unauthenticated read. The registry's reported digest is treated as untrusted the same way archive bytes are — Pull recomputes the digest from the actually-downloaded content and refuses to keep it if the registry didn't report a digest at all, or if the recomputed digest doesn't match what was reported.
- **Import**: every archive entry path is checked with `SafeJoin` before anything is written, and the fully extracted content is re-run through the complete `Validate` pass before it's kept — a full re-check, not a trust-the-archive-because-it-parsed shortcut (ADR 0011). Import never overwrites an existing local package; use `--as` to bring in a second copy under a different name.

None of this proves *who* published a package — v1 has no signing and no publisher-identity verification beyond the registry's own claim-on-first-publish rule. The digest proves content integrity, not authorship (ADR 0011's Follow-Up).

## What `enable`, Setup Prompts, And Materialization Ask Before Writing

- **Enable**: if a package declares setup (tracker files or directories its workflow expects to exist), `lineage enable` shows exactly what would be created — file by file, directory by directory — and asks for confirmation before creating anything, unless `--yes` was passed. Declining setup leaves the workspace completely unchanged, same as declining to enable at all.
- **Materialize**: the first time a given package set would actually change what's staged for a provider, `lineage run`/`lineage workflow run` shows the plan and asks for confirmation (`y`/`yes`, or `--yes` to skip the prompt for scripts). Re-running with an unchanged package set doesn't re-prompt. This is tracked per-provider in `.lineage/materialized-<provider>.json` so cleanup on disable is exact, not guessed from disk contents (ADR 0008).

## Capabilities: Declared, Not Enforced

A package's manifest can carry an optional `capabilities` block (`filesystem.read`, `network`). `lineage package validate` and `--dry-run` print what a package declares wanting. **Nothing in this build enforces, blocks, or warns based on those values beyond printing them** (ADR 0006) — a `capabilities` block that looks unenforced is expected, not a bug. Treat a declared capability as a receiver-visible statement of intent, not a permission you're granting or a boundary Lineage is holding.

## Secret Scanning: What It Catches And What It Doesn't

`internal/packages.ScanForSecrets` checks a small, explicit, documented set of signals (ADR 0009):

- Denylisted filenames: `.env`, `.npmrc`, SSH private key names, `.pem`/`.key`/`.pfx`/`.p12`.
- A short list of high-confidence content patterns: private key headers, AWS access key ID shape, GitHub token prefixes.

Findings report a file path and a human-readable reason only — the matched value itself is never included, so scan output is always safe to print or log. This is a **precise, not exhaustive** scan by design: it will not catch every possible secret shape (custom token formats, secrets embedded in prose, anything that doesn't match the documented list). It's one input `validate`/`export`/`import` consume, not a claim of complete secret safety. Do not treat a clean scan as proof a package contains no sensitive data — see [What Package Authors Should Never Publish](#what-package-authors-should-never-publish) below.

## PII / Personal-Context Detection

**Not implemented in this build.** Lineage has no automated check for personally identifiable information or personal context embedded in package content (a resume, notes referencing real people, personal API usage history, etc.). If a package might contain that kind of content, the author is solely responsible for reviewing it before publishing — `validate`, `export`, and `publish` will not catch it.

## Instruction-Risk Scanning

**Not implemented in this build.** Lineage does not scan skills, workflows, or policy content for prompt-injection-style instructions aimed at an agent reading them (e.g. "ignore your previous instructions and..."). Package content — skills, workflows, agents, policies, references — should be read as untrusted *content*, never as instructions from Lineage itself or from the receiver, but nothing currently flags a package that tries to blur that line. Read what you enable.

## What Package Authors Should Never Publish

Per [SECURITY.md](../SECURITY.md), a package should never include:

- API keys, auth tokens, or provider credentials
- `.env` values
- Shell history
- Local credential stores
- Provider login state
- Private machine-local cache files

If a workflow needs configuration, prefer setup prompts, templates, or explicit user-provided values entered on the receiver's own machine — not baked into the package.

## What Receivers Should Do When They See A Warning

- A **secret-scan finding** blocks validation outright — the package cannot be exported, published, or imported until it's fixed. There's nothing to decide; don't try to force it through.
- A **declared capability** (`filesystem.read`, `network`) is not blocked or gated — read what's declared, and decide for yourself whether you trust this package enough to enable it, the same way you'd read a permissions list before installing anything else.
- A **setup prompt** or **materialization confirmation** lists exactly what will be created before it happens — read the file/directory list, not just the package name, before approving.
- If something doesn't add up — a skill that seems to want more access than its description implies, a policy file with unusual instructions — treat that as a reason not to enable, not something to enable and monitor. Lineage surfaces what it can check; it does not vouch for package intent.

## Related Docs

- [Architecture](architecture.md) — system boundaries and the short version of this page.
- [ADR 0005](decisions/0005-manifest-is-authoritative-with-schema-and-digest.md) — manifest schema and digest.
- [ADR 0006](decisions/0006-capabilities-are-declared-not-enforced.md) — capabilities are declared, not enforced.
- [ADR 0009](decisions/0009-secret-scanning-is-a-documented-list-not-an-engine.md) — secret scanning scope.
- [ADR 0010](decisions/0010-path-safety-only-applies-to-package-controlled-input.md) — path safety scope.
- [ADR 0011](decisions/0011-export-import-treats-archives-as-untrusted-input.md) — export/import trust model.
- [SECURITY.md](../SECURITY.md) — how to report a vulnerability, and what should never be packaged.
