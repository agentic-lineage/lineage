# Compiling An Existing Agent Workspace

Lineage's package compiler is being built for workspaces that already contain
agent behavior but are not yet a clean Lineage package: `CLAUDE.md`,
`AGENTS.md`, `SKILL.md`, scripts, references, setup notes, and informal
workflow instructions.

This is contributor-facing design documentation. There is no public `lineage
package compile` command yet.

## Why Copying Files Is Not Enough

An existing workspace can be structurally complete and still be behaviorally
unclear. A script may be mentioned only from a skill, a setup file may be
implicit, and workflow order may be spread across several instructions. A
portable package must preserve the important behavior, not merely reproduce the
author's folders.

## Current Pipeline

1. **Evidence inventory (#203):** `internal/inventory` walks the source tree
   read-only, classifies files, hashes them, and records literal Markdown
   references between files. It rejects symlinks and skips common dependency,
   build, and cache directories.
2. **Behavioral model (#103):** `internal/model` converts inventory evidence
   into a versioned, provider-neutral `BehavioralModel` — ordered steps, each
   with typed `Claim`s for inputs, outputs, required skills, tools, and
   references, plus setup needs and validation gates. Every claim carries its
   own evidence back into the inventory; unresolved or ambiguous behavior
   becomes an explicit `Decision` rather than a guess. See
   [ADR 0017](../decisions/0017-behavioral-model-is-a-versioned-evidence-linked-schema.md)
   and the mapping to package concepts below.
3. **Agent-assisted analysis (#104):** resolve ambiguity that literal matching
   cannot answer, such as an instruction that says “run the deploy script”
   without naming a path. The agent must report ambiguity rather than guess.
4. **Artifact compilation (#106):** generate a reviewable Lineage package from
   the model, then project it through existing provider adapters.
5. **Portability and behavior validation (#109, #113):** redact or
   parameterize machine-local assumptions and prove that generated artifacts
   still represent modeled workflow steps.

## Behavioral Model To Package Concepts

`internal/model`'s scope is defining this correspondence, not executing it —
#106 (artifact compilation) is what actually generates package content from
it.

| Model field | Manifest concept | Notes |
|---|---|---|
| `Step.Skills[].Value` (deduped across steps) | `Requires.Skills` | Steps are source of truth; #106 flattens/unions the claim values |
| `Step.Tools[].Value` | new `skills/`/`scripts/` package content | No existing manifest field — #106 must materialize these as files |
| `Step.References[].Value` | `references/` package directory | Copied content, cited via the claim's `EvidenceRef.Path` |
| `Step.Setup` | `Setup.Files` / `Setup.Directories` | Field shapes already match (`Path`, `Description`) |
| Whole `Steps` sequence | `WORKFLOW.md` frontmatter `steps:` (`packages.Workflow.Steps`) | `Step.ID`/`Name` becomes the ordered skill-name list `packages.Workflow` expects today |
| `BehavioralModel.Name`/`Intent` | `Manifest.Name`/`Description`, an `Exports.Workflows` entry | |
| `Decision` (unresolved) | none yet | #104 must resolve these (produce/update the `Claim` at `Decision.Refs[i]`); #106 must refuse to compile while unresolved `Decision`s remain |
| `Gate` | none yet | Future validation-runner input (#109/#113) |

## Boundaries

- The inventory is evidence, not a complete call graph or an LLM judgment.
- Analysis must never execute source scripts.
- Unknown or ambiguous files are intentional outputs for later review; they
  must not be silently classified as safe or irrelevant.
- Generated packages still pass the normal validation, digest, import, setup,
  and materialization controls.

See [ADR 0016](../decisions/0016-prioritize-package-distribution-and-behavioral-compilation.md) and [the architecture](../architecture.md) for the current product boundary.
