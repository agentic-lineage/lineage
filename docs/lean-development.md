# Lean Development Process

Lineage uses a lean, goal-based development process.

## Principles

- Work in small, reviewable slices.
- Prefer validated learning over large speculative builds.
- Use issues for concrete action items with acceptance criteria.
- Assign an issue before starting work.
- Link every PR to an assigned issue.
- Keep labels simple. Use milestones and the project board for planning.

## Goal-Based Milestones

Milestones are goals, not dates.

The current program of work is recorded in [ROADMAP.md](../ROADMAP.md). It
prioritizes integration of the existing provider/diagnostics queue, workflow
compilation, receiver trust and lifecycle, and real package adoption. The
GitHub Milestones page remains authoritative for the active issue grouping.

A milestone is done when its issues prove the goal works well enough to trust, not when a calendar date arrives.

Stable releases are cut from completed or explicitly bounded goal milestones.
See [Release And Versioning Policy](release-versioning.md) for how milestone
completion maps to `master` promotions and SemVer tags.

## Issue Shape

Good issues should include:

- The user problem.
- Why it matters.
- Scope.
- Acceptance criteria.
- Safety concerns when relevant.

## Labels

Labels should stay minimal:

- `bug`
- `enhancement`
- `documentation`
- `security`
- `critical`
- `needs:decision`
- `good first issue`
- `help wanted`
- `question`

Avoid creating many area labels. The milestone should explain the goal, and the project board should show workflow status.

## Pull Requests

Every PR should:

- Target `develop`.
- Link an assigned issue.
- Stay small enough to review.
- Include tests or explain why tests do not apply.
- Explain package safety impact when the change affects export, import, setup, or file materialization.
- Check [public-docs-sync.md](public-docs-sync.md) when a change affects install,
  publishing, receiver activation, setup prompts, provider compatibility, or
  safety wording.
- Do not document an open provider PR as supported behavior. Its release docs
  land with the adapter after it is merged into `develop`.
