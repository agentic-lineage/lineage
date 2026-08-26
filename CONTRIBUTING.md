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
- Add tests for package discovery, config changes, launch planning, and any safety checks you touch.

## Agent-Assisted Development

Repository skills in `.agents/skills` are Lineage guardrails, not generic engineering manuals. Use strong external/domain skills for Go, CLI design, security review, and code review when available, then apply the Lineage guardrails to keep work provider-neutral, deterministic, inspectable, permission-gated, and public-safe.

## Issue Ownership

- Assign an issue to yourself before starting work.
- Start work only after the issue is assigned.
- If an issue is already assigned, coordinate with the assignee before opening a competing PR.
- Keep issue discussion focused on the concrete user problem, expected behavior, safety impact, and verification plan.

## Branch Policy

- `main` is the stable branch.
- `develop` is the active development branch.
- Feature and fix branches should target `develop`.
- Changes reach `main` only after they have been reviewed and are ready for a stable release point.
- Protected branches should require tests to pass and owner approval before merge.

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
