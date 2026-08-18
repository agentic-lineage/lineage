# Branch Policy

Lineage uses two protected long-lived branches:

- `master`: stable branch.
- `develop`: active development branch and default PR target.

## Required Flow

1. Create or choose an issue.
2. Assign the issue to yourself before starting work.
3. Create a feature or fix branch from `develop`.
4. Open a PR into `develop`.
5. Link the assigned issue in the PR.
6. Wait for tests and owner review before merge.

PRs without a linked assigned issue should not be reviewed except for maintainer-only housekeeping.

## Recommended GitHub Protection

Apply these protections to both `master` and `develop`:

- Require a pull request before merging.
- Require at least one approving review.
- Dismiss stale approvals when new commits are pushed.
- Require review from code owners.
- Require status checks to pass before merging.
- Require the `Go` test workflow.
- Require branches to be up to date before merging.
- Restrict force pushes.
- Restrict deletions.
- Require conversation resolution before merging.

The test workflow runs for pull requests targeting `develop` and pushes to `develop` or `master`.

## Planning Model

Use goal-based milestones instead of date-based milestones. Labels should stay lightweight; day-to-day planning belongs in the GitHub Project board and milestone views.
