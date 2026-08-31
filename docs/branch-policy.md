# Branch Policy

Lineage uses two protected long-lived branches:

- `main`: stable release branch.
- `develop`: active integration branch and default PR target.

```text
feature/* --PR--> develop
                    |
                    | release PR
                    v
                  main
                    |
                    +-- sync back --> develop
```

## Required Flow

1. Create or choose an issue.
2. Assign the issue to yourself before starting work.
3. Create a feature or fix branch from `develop`.
4. Open a PR into `develop`.
5. Link the assigned issue in the PR.
6. Wait for tests and owner review before merge.

Feature and fix PRs target `develop`. Squash merging is allowed for these PRs
when it keeps the history readable.

Stable promotions are PRs from `develop` to `main`. They must use a merge commit,
not squash merge or rebase, so the release commit can be synchronized back into
`develop` and preserved in branch ancestry.

Hotfix PRs may target `main` only when the stable branch needs an immediate fix.
After a hotfix lands, synchronize `main` back into `develop` before normal
development continues.

PRs without a linked assigned issue should not be reviewed except for maintainer-only housekeeping.

## Recommended GitHub Protection

Apply these protections to both `main` and `develop`:

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

The test workflow runs for pull requests targeting `develop` or `main` and
pushes to `develop` or `main`.

Release tags should point at commits that are already on `main`. See
[Release And Versioning Policy](release-versioning.md) for the tagging,
release-note, and stable-promotion rules.

Every commit introduced into `main` must eventually become an ancestor of
`develop`. After every release or hotfix, verify that `main` has zero unique
commits missing from `develop`.

## Planning Model

Use goal-based milestones instead of date-based milestones. Labels should stay lightweight; day-to-day planning belongs in the GitHub Project board and milestone views.
