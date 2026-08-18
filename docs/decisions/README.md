# Decision Log

This folder records important product and technical decisions for Lineage.

Use this log when a decision will be expensive to reverse, affects contributors, shapes package semantics, or clarifies why the project chose one path over another.

## Format

Decision records use short ADR-style files:

```text
NNNN-short-title.md
```

Each record should include:

- Status
- Context
- Decision
- Consequences
- Follow-up

## Status Values

- `Proposed`: under discussion.
- `Accepted`: current project direction.
- `Superseded`: replaced by a newer decision.

## Decisions

- [0001 Use Go For The Local Runtime](0001-use-go-for-local-runtime.md)
- [0002 Keep Core Provider-Agnostic](0002-keep-core-provider-agnostic.md)
- [0003 Use Goal-Based Milestones](0003-use-goal-based-milestones.md)
- [0004 Require Issue Links And Test Plans For PRs](0004-require-issue-links-and-test-plans-for-prs.md)
