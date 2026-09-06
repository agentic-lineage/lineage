# Release And Versioning Policy

This policy covers Lineage CLI/runtime releases from this repository. Package
versions in `lineage.yaml` are authored package versions and are separate from
Lineage's own release tags.

## Stable Branch

`main` is the stable public baseline. A commit promoted to `main` should be
something a user can install, run, and cite in a bug report.

Development happens on `develop`. Feature and fix work should merge to
`develop` first, then `develop` is promoted to `main` when the current stable
goal is coherent enough to release.

## Version Scheme

Lineage uses SemVer tags for stable releases in the form
`vMAJOR.MINOR.PATCH`.

- `PATCH` releases, such as `v0.3.1`, are for bug fixes, issue fixes,
  documentation corrections, CI fixes, small safety hardening, and
  behavior-preserving improvements.
- `MINOR` releases, such as `v0.4.0`, are for backward-compatible features,
  new commands, new provider support, and new validation checks that do not
  break existing valid packages.
- `MAJOR` releases, such as `v1.0.0` or `v2.0.0`, are for major product
  capability boundaries or breaking compatibility changes in CLI behavior,
  package schema behavior, provider materialization semantics, or runtime
  guarantees.
- Pre-distribution releases use `v0.x.y`. Before `v1.0.0`, Lineage can still
  make faster-breaking changes, but release notes must call out compatibility
  risk plainly.

`v1.0.0` should not be tagged until the `Goal: Enable distribution` milestone is
complete enough that a normal user can install Lineage, publish or fetch a
package, inspect it, enable it, and run it through a local provider workflow.

## Tagging

Every meaningful stable promotion to `main` should have an annotated tag:

```bash
git tag -a v0.x.y -m "Lineage v0.x.y"
git push origin v0.x.y
```

Tags are created manually by a maintainer for now. The maintainer chooses the
version, writes the release notes, creates the annotated tag, pushes it, and
publishes the GitHub Release.

Release automation may later build artifacts from tags, publish checksums, and
draft GitHub release notes, but automation should not choose the version number
or decide that a milestone is complete.

## Release Flow

Stable releases follow this sequence:

```text
develop -> main release PR
        -> merge commit
        -> tag merged main commit
        -> publish GitHub Release and artifacts
        -> sync main back into develop
```

Do not squash-merge or rebase a release promotion. The merge commit on `main`
is part of the release record and must become an ancestor of `develop` during
the sync-back step.

## Release Roles

- Maintainer: decides release scope, chooses the version, creates the annotated
  tag, and publishes release notes.
- Reviewer: confirms that the release notes and version bump match the closed
  issues or milestone scope.
- CI: verifies tests and release-tracking checks before the promotion lands.
- Automation: may build artifacts and checksums later, but does not decide the
  version or release boundary.

## Release Notes

Each GitHub Release should state:

- What works in this release.
- What is experimental.
- What is explicitly not included.
- Known compatibility or safety notes.
- Install artifact names and checksum location when artifacts are attached.

Release notes should link to the closed milestone or issues that define the
release scope.

## Required Checks

Before promoting to `main` or tagging a release:

- All release-scope issues are closed or explicitly deferred.
- The PR from `develop` to `main` passes the `Go` test workflow and release
  tracking check.
- The PR declares the planned SemVer tag, release classification, release notes,
  and post-merge annotated tag plan.
- The release promotion uses a merge commit rather than squash merge or rebase.
- Branch protection requires review, passing status checks, conversation
  resolution, and no force pushes or branch deletion.
- The release notes identify any skipped automated tests or manual validation.
- After the release PR lands, synchronize `main` back into `develop` and verify
  that `main` has zero commits not represented in `develop`.

Release tags should point at commits that are already on `main`.

## Non-Release Housekeeping

A PR to `main` may skip release tagging only when it is explicitly marked as
non-release housekeeping and explains why no user-facing release is needed.
Examples include release-doc typo fixes, workflow metadata cleanup, or
administrative repository maintenance.

## Current Release Boundary

Until `v1.0.0`, releases are allowed to be incomplete as long as their notes are
plain about the boundary. For example, a `v0.x.y` release may support safe local
package export/import without claiming the full hosted distribution flow is
ready.
