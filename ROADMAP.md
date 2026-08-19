# Roadmap

This roadmap is intentionally limited to the public local package runtime.

## Done

- Package creation, enablement, and launch planning.
- Manifest schema versioning, export authority, and content digests.
- Secret scanning and path-traversal protection for package-controlled input.
- `lineage package validate`, `lineage package export`, `lineage package import`.
- Permission-gated materialization: enabling a package actually stages skills
  into a provider's own directory and generates its context file, with an
  explicit confirmation before anything is written.

## Next

- Land workflow execution (`lineage workflow run`), naming an ordered
  sequence of skills a package declares.
- Round out day-to-day CLI coverage: `list`, `disable`, `inspect`, `doctor`.
- Verify the CLI on Windows, including shim generation and binary
  resolution.
- Add an end-to-end integration fixture covering the full author-to-receiver
  flow through the real CLI.

## Later

- Design the `.lineage` artifact (project- and user-level state) as a
  deliberate whole, rather than one field at a time.
- Broaden provider adapter coverage while keeping the core package format
  provider-neutral.
