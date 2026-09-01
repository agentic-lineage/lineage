# Changelog

All notable changes to Lineage will be documented here.

## Unreleased

- Reject explicit schema zero in project configuration and materialization
  state while preserving the legacy default for files with no schema field.

## [1.1.1] - 2026-09-01

### Added

- Add deterministic, read-only source-workspace inventory for the workflow
  compilation pipeline (#203), including file classification, content digests,
  and literal Markdown citation evidence without executing source files.

### Fixed

- Preserve executable file modes when packages are exported and imported.
- Reject an explicit package schema version of zero instead of treating it as
  an implicit default.
- Normalize package references consistently when disabling from a subdirectory.
- Restore restrictive permissions on the stored registry-auth token.
- Add a timeout to registry pull requests so an unavailable registry does not
  leave the CLI waiting indefinitely.
- Normalize an empty enabled-package list when loading project configuration.
- Require confirmation before publishing a package, with an explicit `--yes`
  escape hatch for automation.
- Extend secret scanning to detect ASIA-prefixed AWS credentials, GitHub
  fine-grained personal access tokens, and Google API keys.

### Documentation and verification

- Add the canonical safety model, including current limits around PII and
  instruction-risk detection, rollback pinning, and planned yank semantics.
- Document `.lineage` container versioning and the accepted concurrent-write
  limitation.
- Run the Go test matrix on Windows and add receiver-flow coverage for
  materialization and provider path behavior.
- Establish `main` as the stable release branch, with release promotions from
  `develop` and mandatory sync-back after stable releases or hotfixes.

## Historical entries

The entries below predate structured per-version headings. They remain as a
record of earlier Lineage work and are not all part of v1.1.1.

- Initial local agent package runtime scaffold.
- Refreshed public-facing documentation for the current registry, `lineage add`,
  workflow, inspect/list/doctor, and bootstrap-prompt surfaces; added a public
  docs sync checklist for README/Wiki/website/package-page drift.
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
- `internal/provider` now has a single provider registry (`Known`, `Get`,
  `IsKnown`) naming every supported provider and where it reads staged
  content. `internal/runtime` and the CLI's usage text consult it instead
  of hardcoding `claude`/`codex`, so adding a provider is a one-entry
  change in one file rather than a hunt through the core runtime.
- `lineage run claude` now materializes enabled packages for Claude Code:
  skills are staged into `.claude/skills/<pkg>-<skill>/` and a generated
  section in `CLAUDE.md` lists active packages, agents, policies, and
  workflows. Materialization is idempotent and reversible (re-running
  reflects the current enabled-package set exactly, via a per-provider
  `.lineage/materialized-<provider>.json` state file) and is skipped
  entirely on `--dry-run`.
- `lineage run codex` materializes the same way, into
  `.agents/skills/<pkg>-<skill>/` and a generated section in
  `AGENTS.md`. Claude and Codex materialization stay isolated from
  each other (separate state files, separate staged directories) so
  running one provider never disturbs the other's staged content.
- `lineage run` now shows what materialization would create or change
  and asks for confirmation the first time a given package set would
  actually write anything, instead of writing files as a silent side
  effect. Approve with `y`/`yes`, or skip the prompt with `--yes`/`-y`
  for scripts. Re-running with an unchanged package set doesn't
  re-prompt. `lineage.Execute` now takes a `stdin io.Reader` to support
  this.
- Added `lineage package export <path> [-o file.tgz]`: produces a
  deterministic tar.gz archive (manifest + content directories, sorted
  order, normalized permissions and timestamps) after running the same
  checks as `lineage package validate` and refusing to export if any
  fail. Two exports of byte-identical package content always produce
  byte-identical archive bytes.
- Added `lineage package import <file.tgz> [--as name]`: extracts an
  exported archive into the user packages directory. The archive is
  treated as untrusted input — every entry path is checked against
  path traversal before anything is written, and the fully extracted
  content is run through the same checks as `lineage package validate`
  before it's kept, so a broken or unsafe archive is discarded rather
  than installed. Never overwrites an existing package; use `--as` to
  import under a different name. Export followed by import reproduces
  the original package exactly, verified by matching content digests.
- Vendored `gopkg.in/yaml.v3`, Lineage's one dependency, under
  `vendor/`. `go build`/`go test ./...` no longer need network access
  to `proxy.golang.org` on a fresh checkout — Go automatically prefers
  the vendored copy whenever `vendor/modules.txt` is present and
  consistent with `go.mod`.
- Added `lineage list` (enabled packages in the current project, with
  digest), `lineage disable <ref>` (removes a package and
  re-materializes for every provider that has ever run in this
  project, so disabling actually cleans up staged skills instead of
  just editing config and leaving stale files behind), and `lineage
  inspect <ref>` (manifest, discovered contents, digest, and
  capabilities for any resolvable package — project path, user id, or
  workspace id — without enabling it).
- Fixed a pre-existing rendering bug where `lineage help`/usage output
  contained literal tab characters instead of consistent indentation.
- Added `lineage doctor`: checks project config validity (and that
  every enabled ref still resolves), whether the shim directory is on
  `PATH` and ordered before real provider binaries (a real binary
  found first means the shim never takes effect), and warns when more
  than one candidate binary exists for a provider on `PATH` — naming
  every candidate, not just the one that currently wins silently.
  Fails (non-zero exit) only for things that are actually broken;
  ambiguous-but-working situations are warnings.
- `lineage install-shims` now generates a `.cmd` batch shim on Windows
  (previously it only ever wrote POSIX `sh` scripts, which Windows
  can't execute) and a POSIX shim everywhere else, and shims are
  generated from `internal/provider`'s registry instead of a
  hardcoded `claude`/`codex` list. Provider binary resolution
  (`findRealBinary`/`CandidateBinaries`) now resolves real binaries on
  Windows through `PATHEXT` (`.exe`, `.cmd`, etc.) instead of only
  matching an exact, extension-less filename.
- `WORKFLOW.md` can now declare an ordered `steps` list (YAML
  frontmatter, the same convention `SKILL.md` already uses) naming
  skills within the same package. `lineage package validate` checks
  every step resolves to a real skill.
- Added `lineage workflow run <workflow-name> <provider> [--dry-run]
  [--yes] [-- provider args...]`: finds which enabled package declares
  the workflow and materializes *only* its steps — not the full
  enabled package set — in order, then hands off to the provider. The
  provider's generated context file explicitly lists the active
  workflow and its ordered steps. Same permission-gate/`--dry-run`
  behavior as `lineage run`; a plain `lineage run` afterward correctly
  restores the full enabled package set, since both share the same
  per-provider materialization state.
- Fixed a rendering bug where `lineage help`/usage output for several
  commands contained literal tab characters instead of consistent
  indentation.
- Added `lineage package publish <path>` and `lineage package pull
  <package-ref> [--as name]` against the Lineage registry (the
  `landing/` website's `api/`, backed by a private GitHub repo used
  purely as artifact storage — see `docs/decisions/
  0012-v1-distribution-contract-and-receiver-activation.md`). Publish
  reuses the same Validate-then-Export path as `package export` and
  refuses to publish anything that fails validation; publishing the
  same `name@version` twice with identical content is a no-op,
  publishing it with different content is rejected — published
  versions are immutable. Pull resolves a ref (`name` for latest, or
  an exact `name@version`), imports it exactly like `package import`,
  and then independently recomputes the content digest from what was
  actually imported, failing closed and discarding the import if it
  doesn't match what the registry reported. Registry location and the
  publish token are read from `LINEAGE_REGISTRY_URL`/
  `LINEAGE_PUBLISH_TOKEN`, not `.lineage/config.yaml` — a publish
  credential is a per-invocation secret, not committed project state.
- The registry now records who published each package (a publish token
  maps to a publisher id) and enforces that only the same publisher can
  push a new version of a name they already own — publishing an update
  is a normal `lineage package publish` with a bumped `version` and the
  same token; a different publisher's token is rejected.
- Publish is now safe to interrupt and retry: since publish is commonly
  run by an agent on someone's behalf and that agent's own session can
  end mid-command, the registry creates the GitHub Release as a draft,
  uploads the archive, and only then finalizes it — a publish cut off
  in between is invisible to receivers and resumed, not duplicated or
  errored on, by simply retrying the same publish call. CLI-side error
  messages from a failed publish/pull now surface the registry's own
  error text instead of a raw JSON body.
- `lineage enable` now records a local lineage graph entry
  (`.lineage/graph.json`) noting which package a project's state
  descends from; `lineage graph list [--yaml]` shows the recorded
  history for the current project.
- Added a content-addressed, immutable snapshot store
  (`~/.lineage/objects/`, `~/.lineage/snapshots/`): `lineage enable` now
  also takes a durable snapshot of exactly what was enabled, and links
  it to the graph entry above via `snapshot_id`. Identical file content
  is stored once regardless of how many packages/versions reference it.
- Added `docs/safety.md` as the canonical safety model (#143, #137):
  maps every safety check to its pipeline stage, spells out secret-scan
  and capability-declaration limits, and says plainly that PII/personal-
  context detection and instruction-risk scanning are not implemented
  yet. README, `docs/architecture.md`, `docs/guides/lineage-package.md`,
  `AGENTS.md`, `SECURITY.md`, and `llms.txt` now link to it.
- Documented the exact-version pinning flow as the current rollback path
  (#144) and the planned yank/tombstone semantics for `lineage package
  unpublish` (#145) in the README, ahead of either command landing.
- The `.lineage` directory is now designed as a whole (#59, ADR 0015):
  `.lineage/config.yaml` and `.lineage/materialized-<provider>.json` carry
  a `schema` field, defaulted for existing files and rejected if
  unsupported, matching `lineage.yaml`'s existing convention. `lineage
  enable` now gitignores a project's `.lineage/` the first time it
  creates one, since it can carry machine-local provider paths and
  regenerable cache that should never be committed.
