# Hypothesis And Test Plan

## Hypotheses

- A package can preserve a useful agent environment without requiring users to rewrite it in a new workflow language.
- A receiver can inspect package contents before enabling them.
- Package enablement can be idempotent and safe for repeated runs.
- Provider-specific launch behavior can stay behind explicit adapter boundaries.
- A package can move to another machine (export, then import) and reconstruct exactly, without the receiver having to trust the archive's origin.

## Current Tests

- Project config load, save, and discovery (`internal/config`).
- Package initialization, manifest loading, and discovery, including
  export-authoritative resolution and schema versioning
  (`internal/packages`).
- Path traversal rejection for package-controlled input
  (`internal/packages/safepath_test.go`).
- Secret-shaped filename and content detection
  (`internal/packages/secrets_test.go`).
- `lineage package validate` end to end, including a failing case
  (`internal/cli/cli_test.go`).
- Export/import archive round trip and exact content reconstruction,
  including a byte-identical digest check and rejection of a package that
  fails validation (`internal/packages/export_test.go`,
  `internal/packages/import_test.go`).
- Materialization staging and its idempotent, per-provider state tracking,
  for both the Claude Code and Codex adapters (`internal/materialize`).
- The permission-gated confirmation flow before materialization writes
  anything, and that `--dry-run` never triggers it (`internal/cli`).
- Provider launch planning and command resolution (`internal/provider`).
- Runtime plan rendering (`internal/runtime`).
- Shim creation (`internal/shim`).

## Needed Tests

- A single end-to-end integration fixture exercising the full
  author-to-receiver flow (package, export, import, enable, run,
  materialize, disable) through the real CLI entrypoint, once workflow
  execution and the remaining CLI commands land.
- Real Windows CI verification for shim generation and binary resolution
  (currently covered by GOOS-parameterized unit tests only, run on
  non-Windows CI).
