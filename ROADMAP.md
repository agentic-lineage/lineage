# Roadmap

This roadmap is intentionally limited to the public local package runtime.

## Done

- Package creation, enablement, and launch planning.
- Manifest schema versioning, export authority, and content digests.
- Secret scanning and path-traversal protection for package-controlled input.
- `lineage package validate`, `lineage package export`, `lineage package import`.
- Registry publish and pull backed by the Lineage website API.
- `lineage add` as the one-command receiver path for published packages,
  converging local `.tgz` archives and registry refs through the same
  validate → inspect → setup → enable flow, idempotent on repeat.
- GitHub device-flow login, logout, and publisher identity checks.
- `lineage list`, `lineage disable`, `lineage inspect`, and `lineage doctor`.
- Workflow execution through `lineage workflow run`.
- Claude and Codex provider materialization, dry-run previews, and local shims.
- Permission-gated materialization: enabling a package actually stages skills
  into a provider's own directory and generates its context file, with an
  explicit confirmation before anything is written.
- Package-declared setup files/directories, created through their own
  explicit permission-gated prompt.
- Provider entrypoints and capabilities surfaced in the registry and on
  package pages.
- A portability report (blockers/warnings) computed during export and
  publish.
- The `.lineage` directory (project- and user-level state) designed as a
  deliberate whole — every file enumerated, schema versioning decided,
  a gitignore default, and a regenerable-vs-authoritative classification
  (#59, ADR 0015).

## Next

An August 2026 consolidation pass audited every shipped surface (CLI,
package lifecycle, materialization/runtime, and the registry backend) for
correctness and security bugs rather than new features, and filed the
result as individual issues instead of one grab-bag. That work takes
priority over new feature surface:

- Critical/high fixes: unvalidated package names enabling path escapes
  (#147), digest verification skipped on an empty registry response (#148),
  a stdin-buffering bug that silently drops the second of two prompts in
  one command (#149), `enable` persisting state before a later step that
  can fail (#150), secret scanning gaps (#151, #152), `lineage add`
  reporting success when nothing was enabled (#153), and no write path in
  the codebase being crash-safe (#154).
- The rest of that audit's medium/low findings are filed individually
  (#155-#172) so they can be picked up independently; several are tagged
  `good first issue`.
- Add a Windows CI job (#174) so the shim `.cmd` generation and `PATHEXT`
  binary-resolution code this repo already ships actually run somewhere
  before merge, instead of only on ubuntu-latest.
- Add an end-to-end integration fixture covering the full author-to-receiver
  flow through the real CLI (#12).
- Keep public docs synchronized across README, Wiki, Discussions, and the
  website whenever the receiver flow changes.
- Add automated drift checks for the bootstrap prompt copy embedded on package
  pages.

## Later

- Broaden provider adapter coverage (Cursor, Windsurf, Auggie, Cline, Aider,
  GitHub Copilot) while keeping the core package format provider-neutral.
- Add stronger capability enforcement if declarative capability visibility
  is not enough for real receiver trust.
- The "safety compliance" (#127-#142) and "compile existing workflows into
  packages" (#101-#116) epics are deliberately parked here rather than in
  Next: both are net-new feature surface, and the project's stated priority
  right now is consolidating what's already shipped, not expanding it
  further.
