## Summary

What changed, and why?

## Linked Issue

Closes #

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

## Safety

- [ ] This change does not export secrets, credentials, private prompts, or machine-local state.
- [ ] Package inputs are treated as untrusted.
- [ ] Path handling prevents traversal outside the intended workspace/package root where applicable.
- [ ] Provider-specific logic stays behind an explicit boundary.

## Verification

List the relevant test cases added or updated, then paste the commands or checks you ran.

Relevant test cases:

- 

```text
go test ./...
```

If this PR does not need automated tests, explain why:

- 

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

-

## Notes For Reviewers

Call out compatibility concerns, follow-up work, or areas that need extra scrutiny.
