# Contributing To Lineage

Lineage is early, so the best contributions are focused, testable, and careful about package safety.

## Local Setup

```bash
go test ./...
go run ./cmd/lineage --help
```

No network access is required: Lineage's one dependency (`gopkg.in/yaml.v3`) is vendored under `vendor/`, and Go automatically prefers it over the module proxy whenever `vendor/modules.txt` is present and consistent with `go.mod`. If you add or bump a dependency, run `go mod vendor` afterward and commit the resulting `vendor/` changes alongside `go.mod`/`go.sum`.

## Development Guidelines

- Keep the runtime provider-neutral unless you are working inside an explicit provider boundary.
- Treat package contents as untrusted input.
- Keep manifests deterministic and human-readable.
- Do not add code or docs that encourage sharing secrets, credentials, provider login state, or private machine state.
- Treat source-workspace inventory as evidence: do not execute source scripts or turn ambiguous input into asserted behavior.
- Add tests for package discovery, config changes, launch planning, and any safety checks you touch.

## The `.lineage` Directory

A project's `<project>/.lineage/` (config, materialized provider state, the local lineage graph) is machine-local and regenerable-or-historical, never portable package content — it must never be committed to a receiver's repo. `lineage enable` gitignores it automatically and reinforces the idempotent `.lineage/` entry on later enables. If you add a new file under project- or user-level `.lineage/`, enumerate it and classify it (regenerable vs. authoritative) in [docs/decisions/0015](docs/decisions/0015-the-lineage-directory-is-a-versioned-enumerated-container.md) rather than leaving it undocumented, and give it a `schema` field if its shape could plausibly need to change incompatibly later (see ADR 0005's precedent).

## Agent-Assisted Development

Repository skills in `.agents/skills` are Lineage guardrails, not generic engineering manuals. Use strong external/domain skills for Go, CLI design, security review, and code review when available, then apply the Lineage guardrails to keep work provider-neutral, deterministic, inspectable, permission-gated, and public-safe.

## Issue Ownership

- Assign an issue to yourself before starting work.
- Start work only after the issue is assigned.
- If an issue is already assigned, coordinate with the assignee before opening a competing PR.
- Keep issue discussion focused on the concrete user problem, expected behavior, safety impact, and verification plan.

## Branch Policy

- `main` is the stable release branch.
- `develop` is the active integration branch and default PR target.
- Feature and fix branches target `develop`.
- Stable releases are promoted through a pull request from `develop` to `main`.
- Release promotions use a merge commit rather than squash merging.
- After a release promotion lands, `main` is synchronized back into `develop`
  before normal development continues.
- Hotfixes that land on `main` must also be synchronized back into `develop`.
- Protected branches require tests and the configured review policy before merge.

## Lean Planning

Lineage uses goal-based milestones rather than date-based milestones. A milestone represents a product learning goal, such as proving safe package round trips or receiver setup.

Keep labels minimal. Use labels for important signals like `bug`, `enhancement`, `documentation`, `security`, `critical`, or `needs:decision`; use milestones and the project board for planning.

## Pull Requests

Use the pull request template and include verification output. Every PR must link an assigned issue. PRs without a linked issue should not be reviewed except for maintainer-only housekeeping changes.

For behavior that affects package setup or local files, explain what a receiver can inspect before enabling it.

For changes that affect install, publishing, receiver activation, setup prompts,
provider compatibility, or safety wording, check
[docs/public-docs-sync.md](docs/public-docs-sync.md) and call out any website,
Wiki, package-page, or Discussion sync still needed.

For inventory or behavioral-compilation changes, also update the
[compilation guide](docs/guides/compiling-existing-workspaces.md) and keep the
evidence/interpretation boundary explicit.
