# 0005 Manifest Is Authoritative, With Schema Versioning And A Content Digest

Status: Accepted

Date: 2026-08-19

## Context

Phase 1 made enabling a package materialize real content for a provider. Before the export/import loop let that content move between machines, the package format had several open questions that would become expensive once packages were shared: whether `lineage.yaml` or the filesystem wins when they disagree about exports, whether a future manifest change can signal "old parsers can't read this," and whether a package reference like `name@version` means anything verifiable.

## Decision

- The manifest is authoritative over the filesystem for declared exports. When `exports.agents`/`exports.workflows` is non-empty, every declared name must exist on disk (missing is a `Discover`-time error) and only declared names count as exported; anything present but undeclared stays local but is excluded. An empty/omitted list falls back to full filesystem discovery, so packages predating this decision are unaffected.
- `lineage.yaml` carries a `schema` integer, separate from the package's own `version`. `schema` describes how to interpret the manifest; `version` describes the package. A missing `schema` (every manifest before this decision) defaults to 1; anything else is rejected as unsupported rather than silently misparsed.
- Every discovered package carries a `sha256` content digest over the manifest and its standard content directories, computed in deterministic order, so `name@version` can be paired with "and this is exactly what that resolved to."

## Consequences

Package identity becomes verifiable instead of just a string. Contributors adding manifest fields must decide up front whether they're export-authoritative content or advisory metadata. A future incompatible manifest format has a clean way to say so (`schema: 2`) instead of a parser guessing.

## Follow-Up

The digest remains unsigned. For registry packages, the website verifies the
publisher's GitHub identity and binds that identity to the package name; a local
archive still does not establish who sent it. `lineage package validate` stays
the place that surfaces the digest and manifest problems together, rather than
leaving each check only in a library function. See ADR 0011 and ADR 0012 for
the archive and registry trust boundaries.
