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
- `internal/provider` now has a single provider registry (`Known`, `Get`,
  `IsKnown`) naming every supported provider and where it reads staged
  content. `internal/runtime` and the CLI's usage text consult it instead
  of hardcoding `claude`/`codex`, so adding a provider is a one-entry
  change in one file rather than a hunt through the core runtime.
