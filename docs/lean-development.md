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

Current milestones (see the GitHub Milestones page for the authoritative, up-to-date list):

- `Goal: Durable runtime foundations`
- `Goal: Safe package round trip`
- `Goal: Receiver setup experience`
- `Goal: Provider and CI confidence`
- `Goal: Package materialization`
- `Goal: Add providers`
- `Goal: Workflow execution`
- `Goal: CLI completeness`

A milestone is done when its issues prove the goal works well enough to trust, not when a calendar date arrives.

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
