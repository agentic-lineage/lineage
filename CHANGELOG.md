# Changelog

All notable changes to Lineage will be documented here.

## Unreleased

- Initial local agent package runtime scaffold.
- Project, user, and workspace package configuration.
- Package initialization and discovery.
- Provider launch planning and local shims.
- Open-source contribution templates and repository skills.
- Fix: `lineage enable` now walks up to an existing project root (like
  `lineage run` already does) instead of creating a second, shadow
  `.lineage/config.yaml` when run from a subdirectory; relative package
  refs are re-expressed against the project root so they keep resolving
  correctly.
- Fix: `lineage run <provider> ... -- <args>` now truly passes everything
  after `--` through to the wrapped provider, even a literal `--dry-run`,
  instead of always intercepting it as lineage's own flag.
- Fix: `lineage package init` no longer resets an existing `lineage.yaml`
  to defaults when run again against an already-initialized package,
  preserving hand-edited version/description and any existing contents.
- `lineage.yaml` now carries a `schema` field (manifest format version,
  separate from the package's own `version`) and an optional, purely
  declarative `capabilities` block (`filesystem.read`, `network`).
  Existing manifests without either field keep working unchanged.
- Manifest exports are now authoritative when declared: an
  `exports.agents`/`exports.workflows` entry that's declared but missing
  on disk fails discovery with a clear error; content present on disk but
  not declared stays local but is excluded from what's advertised. An
  empty/omitted export list still falls back to discovering everything on
  disk, so undeclared packages are unaffected.
- `requires.skills` is now enforced at `enable` and `run` time against the
  full set of packages that would be enabled together — a missing
  required skill fails with a clear error naming it, instead of silently
  proceeding.
- Every discovered package now carries a stable `sha256` content digest
  (manifest + all content files, deterministic order), surfaced in
  `--dry-run` output alongside any declared capabilities.
- Added `internal/packages.SafeJoin`, a path-traversal guard for any path
  that comes from untrusted, package-controlled input (as opposed to a
  path the user typed directly). `entrypoints.claude`/`entrypoints.codex`
  are now validated against it at discovery time, and package content
  digests refuse to follow a symlink rather than silently hashing
  whatever it points at outside the package.
- Added `internal/packages.ScanForSecrets`, a lightweight scan for
  credential-shaped files and content (`.env`, private key files/headers,
  AWS/GitHub token shapes) across a package directory. Findings report a
  path and reason only — never the matched value — so results are safe to
  print.
- Added `lineage package validate <path>`: runs the manifest schema check,
  export-authority check, entrypoint path safety, and secret scan against
  a package without enabling it or writing anything, printing the
  resolved digest and declared capabilities. Unlike normal package
  resolution, it collects every problem found rather than stopping at the
  first one, and exits non-zero on failure.
