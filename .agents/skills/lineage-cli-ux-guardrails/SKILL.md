---
name: lineage-cli-ux-guardrails
description: Use when adding or reviewing Lineage CLI commands, flags, prompts, help text, dry-run output, shims, or receiver setup flows. Apply general CLI design skill first, then enforce these Lineage-specific UX constraints.
---

# Lineage CLI UX Guardrails

Lineage should make agent-environment distribution feel safe for non-expert users without hiding what will happen.

## CLI Contract

- Prefer explicit verbs: `package init`, `package validate`, `enable`, `run`, `install-shims`.
- Preserve existing command signatures unless the linked issue explicitly calls for a breaking change.
- `--dry-run` must be side-effect-free.
- Interactive prompts must have a non-interactive path for scripts and CI.
- Declining a permission prompt must stop the unsafe operation cleanly.
- Print human-readable summaries by default; keep output deterministic and stable enough to test.
- Send normal command results to stdout and actionable errors to stderr.

## Receiver Experience

- Inspect before enable.
- Show what package, digest, capabilities, provider, files, and setup actions are involved.
- Do not ask users to manually assemble plugin/config/tracker files when Lineage can safely prepare templates for them.
- Do not make activation depend on secrets being bundled. Receiver-local secrets and provider login remain local.
- Avoid jargon in user-facing messages. Explain the action, target path, and consequence.

## Testing Expectations

- Test help/usage for new commands.
- Test successful output and failure output.
- Test `--dry-run`, declined prompts, approved prompts, and repeated runs when the command writes files.
- Test provider argument passthrough when launch behavior changes.
