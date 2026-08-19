---
name: lineage-open-source-review-guardrails
description: Use when creating or reviewing Lineage issues, pull requests, contribution docs, release notes, labels, milestones, project-board updates, or maintainer decisions. Apply general code-review/contribution practice first, then enforce these Lineage-specific open-source rules.
---

# Lineage Open Source Review Guardrails

Lineage should be easy to contribute to without letting quality, safety, or roadmap clarity drift.

## Contribution Rules

- Contributors should assign an issue before starting work.
- PRs should target `develop` and link the assigned issue, except maintainer-only housekeeping.
- Every PR should include relevant tests added/updated, or a clear no-test reason.
- Keep project-board status simple: `Todo`, `In Progress`, `Under Review`, `Done`.
- Milestones are goals, not dates.
- Keep labels small and meaningful: `bug`, `enhancement`, `documentation`, `security`, `critical`, `needs:decision`.

## Review Standard

Review for correctness first, then safety, tests, maintainability, and documentation.

Look especially for:

- package inputs treated as trusted;
- provider-specific behavior leaking into core;
- missing tests for safety or idempotency;
- CLI output that cannot be inspected or tested;
- docs that imply future capabilities are already implemented;
- accidental disclosure of private roadmap, secrets, credentials, or machine-local state.

## Public Voice

- Keep public docs focused on the agent-environment distribution layer.
- Avoid overpromising marketplace, enterprise, billing, cloud, vector DB, or workflow-engine capabilities.
- Prefer plain explanations of what currently works, what is experimental, and what is intentionally out of scope.

## Maintainer Checklist

Before merge:

- Linked issue is assigned or the PR is explicit maintainer housekeeping.
- Required checks pass.
- Tests match the risk of the change.
- Branch target is `develop`.
- Safety checklist is credible, not just checked.
