# Contributing To Lineage

Lineage is early, so the best contributions are focused, testable, and careful about package safety.

## Local Setup

```bash
go test ./...
go run ./cmd/lineage --help
```

## Development Guidelines

- Keep the runtime provider-neutral unless you are working inside an explicit provider boundary.
- Treat package contents as untrusted input.
- Keep manifests deterministic and human-readable.
- Do not add code or docs that encourage sharing secrets, credentials, provider login state, or private machine state.
- Add tests for package discovery, config changes, launch planning, and any safety checks you touch.

## Issue Ownership

- Assign an issue to yourself before starting work.
- Start work only after the issue is assigned.
- If an issue is already assigned, coordinate with the assignee before opening a competing PR.
- Keep issue discussion focused on the concrete user problem, expected behavior, safety impact, and verification plan.

## Branch Policy

- `master` is the stable branch.
- `develop` is the active development branch.
- Feature and fix branches should target `develop`.
- Changes reach `master` only after they have been reviewed and are ready for a stable release point.
- Protected branches should require tests to pass and owner approval before merge.

## Lean Planning

Lineage uses goal-based milestones rather than date-based milestones. A milestone represents a product learning goal, such as proving safe package round trips or receiver setup.

Keep labels minimal. Use labels for important signals like `bug`, `enhancement`, `documentation`, `security`, `critical`, or `needs:decision`; use milestones and the project board for planning.

## Pull Requests

Use the pull request template and include verification output. Every PR must link an assigned issue. PRs without a linked issue should not be reviewed except for maintainer-only housekeeping changes.

For behavior that affects package setup or local files, explain what a receiver can inspect before enabling it.
