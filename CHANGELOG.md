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
