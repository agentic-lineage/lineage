# Hypothesis And Test Plan

## Hypotheses

- A package can preserve a useful agent environment without requiring users to rewrite it in a new workflow language.
- A receiver can inspect package contents before enabling them.
- Package enablement can be idempotent and safe for repeated runs.
- Provider-specific launch behavior can stay behind explicit adapter boundaries.
- A package can move to another machine (export, then import) and reconstruct exactly, without the receiver having to trust the archive's origin.
- A messy agent workspace can be inventoried deterministically without
  executing or mutating its source files.

## Current Tests

- Project config load, save, and discovery.
- Package initialization, manifest loading, and discovery.
- Manifest schema versioning and export authority (`internal/packages/manifest_test.go`, `discovery_test.go`).
- Package content digest: stable across identical content, sensitive to any change, symlinks refused (`discovery_test.go`).
- Path traversal rejection for package-controlled input — manifest entrypoints and archive entries (`internal/packages/safepath_test.go`, `import_test.go`).
- Secret and credential detection by filename and content pattern, never leaking the matched value (`internal/packages/secrets_test.go`).
- `lineage package validate` collecting every problem in one pass, including broken workflow step references (`validate_test.go`).
- Export/import archive round trip: byte-identical exports of identical content, and export → import → `Discover()` reproducing the original package's content digest exactly (`export_test.go`, `import_test.go`, plus a full CLI-level round trip in `internal/cli/cli_test.go`).
- Permission-gated materialization: the confirmation prompt, `--yes` bypass, and declining aborting the whole run rather than partially applying (`internal/cli/cli_test.go`).
- Repeated enablement and materialization idempotency: re-running never duplicates staged content, and a shrinking package set is cleaned up exactly (`internal/materialize/materialize_test.go`).
- Workflow execution scoped to only a workflow's declared steps, and reversibility with a plain full `lineage run` afterward (`internal/materialize/workflow_test.go`).
- Provider launch planning, and a single provider registry with no hardcoded provider names outside `internal/provider` (`internal/provider/registry_test.go`).
- Cross-platform path/shim behavior on both Ubuntu and Windows CI, including
  Windows shim generation and `PATHEXT` binary resolution (`.github/workflows/test.yml`).
- Shim creation and `lineage doctor` diagnostics (config validity, shim `PATH` placement, ambiguous provider binaries).
- Runtime plan rendering.
- End-to-end receiver flow through the real CLI: package → export → import →
  inspect → enable → run → workflow run → disable → doctor
  (`tests/receiver_flow_test.go`).
- Source-workspace inventory: deterministic results, ignored directories,
  symlink refusal, file classification, self-describing literal citations,
  path-boundary handling, and messy-workspace fixtures
  (`internal/inventory/*_test.go`).

## Needed Tests

- Behavioral-model, compiler, portability, and model-to-artifact validation
  coverage for the workflow-compilation pipeline (#103–#116).
- End-to-end registry coverage against a disposable test registry; the current
  receiver fixture covers the real CLI's local archive path.
