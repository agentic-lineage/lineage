## Summary

Briefly explain what changed and why this PR is needed.

## Linked Issue

Closes #123

For maintainer-only housekeeping with no issue, write:

Maintainer housekeeping: <reason>

- [ ] The linked issue is assigned to me.
- [ ] This PR targets `develop`, or it is a release-tracked promotion into `main`.
- [ ] This PR is not ready for review until the linked issue and test plan are complete.

## Package Impact

- [ ] Package format or manifest behavior
- [ ] Package discovery or enablement
- [ ] Provider launch/shim behavior
- [ ] Setup or permission flow
- [ ] Documentation only
- [ ] Tests only

## Merge Method

- [ ] Squash merge for an ordinary PR into `develop`.
- [ ] Merge commit for a release promotion into `main`.
- [ ] Merge commit for a `main` to `develop` sync-back.

## Safety

- [ ] This change does not export secrets, credentials, private prompts, or machine-local state.
- [ ] Package inputs are treated as untrusted.
- [ ] Path handling prevents traversal outside the intended workspace/package root where applicable.
- [ ] Provider-specific logic stays behind an explicit boundary.

## Verification

List the relevant test cases added or updated, then paste the commands or checks you ran.

Relevant test cases:

- None yet.

```text
go test ./...
```

If this PR does not need automated tests, explain why:

- Automated tests apply unless this is documentation-only or maintainer housekeeping.

## Release Tracking

Complete this section only for PRs targeting `main`.

- [ ] Release-worthy main promotion
- [ ] Non-release housekeeping

Planned version:

`v0.0.0`

Release classification:

`patch | minor | major`

Release notes:

What changed:

-

What is experimental:

-

What is outside scope:

-

Safety and compatibility notes:

-

Post-merge tag plan:

- [ ] Create and push an annotated SemVer tag after merge to `main`.
- [ ] Publish the matching GitHub Release and artifacts.
- [ ] Sync the resulting `main` merge commit back into `develop`.

Non-release housekeeping reason:

- Not applicable.

## Notes For Reviewers

Call out compatibility concerns, follow-up work, or areas that need extra scrutiny.
