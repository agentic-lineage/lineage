# 0014 Content-Addressed Snapshot Store Separates Objects From Manifests

Status: Accepted

Date: 2026-08-24

## Context

Issue #7 asks for durable, verifiable package snapshots that can be inspected, copied, and reconstructed without hidden mutable state. The closest existing thing, `packages.ComputeDigest` (ADR 0005), is a single whole-package hash recomputed from disk on demand — it proves two directories are byte-identical, but nothing is ever stored, there's no per-file identity, and there's nothing to copy or reconstruct from. Export/Import (ADR 0011) is a real snapshot-adjacent mechanism, but it's a transport concern — one flat, deterministic tar.gz archive — not a local, deduplicated, addressable store.

This landed in the same PR as issue #6 (the local lineage graph, ADR 0013), which reserved a `SnapshotID` field specifically for this store to fill in — the two issues are deliberately implemented and merged together.

## Decision

- A new package, `internal/snapshot`, is a content-addressed object store: `ObjectID` is always `"sha256:<hex>"` (the same digest format `ComputeDigest` already uses), and it *is* the hash of the object's own bytes — identical content always produces the same ID, so writing the same content twice is a free, correct no-op (deduplication falls out of the design rather than needing separate logic).
- File objects and snapshot manifests are stored in two separate namespaces under the Lineage home directory — `objects/` and `snapshots/`, both fanned out `<hash[:2]>/<hash[2:]>` git-style — per the issue's explicit scope bullet. They share the same addressing scheme and the same low-level `putBlob`/`getBlob` implementation, but a manifest's ID and a file's ID are never comparable/interchangeable by accident because they live in different directories.
- A snapshot `Manifest` is a plain Go struct (`Schema`, `Name`, `Version`, and a `Files` slice kept sorted by path) — the same "struct + sorted slice ⇒ deterministic marshaling" trick `packages.Manifest` already relies on, so two `Create` calls against unchanged content always produce byte-identical manifest JSON and therefore the same manifest `ObjectID`. `Schema` follows ADR 0005's precedent (a format-version field independent of the package's own `version`) rather than the store inventing a different versioning convention.
- `Materialize` verifies every object a manifest references *before* writing anything to the destination directory. A single corrupt object fails the whole call with nothing written, rather than reconstructing a package with silently wrong or missing content — extending ADR 0011's "verify before trusting reconstructed content" idea from whole-archive granularity down to individual objects.
- `Create` reuses `packages.ContentFiles` (newly exported as a thin wrapper over the existing unexported `contentFiles`) so the store's notion of "a package's content" never drifts from what `ComputeDigest`/`Export` already agree on, and reuses `packages.SafeJoin` for writing reconstructed paths, per ADR 0010's existing scope for package-controlled input.
- Wired into `enableRef` in the same commit as #6's graph write: enabling a package now also calls `snapshot.Create` and records the resulting `ObjectID` on the graph entry's `Parent.SnapshotID`, so a graph record and a reconstructable snapshot exist together from day one instead of the field staying empty until some future PR.

## Consequences

An enabled package now has a durable, independently-verifiable copy of its exact content sitting in the Lineage home directory, addressable by a value already recorded in the local lineage graph — inspecting or reconstructing "what a project's state actually was" no longer depends on the original package directory still existing unchanged. Deduplication is free: two packages (or two versions of the same package) sharing a file only store it once. The cost: every `enable` now does a full read of the package's content a second time (once for `ComputeDigest`, again for `Create`) and writes it into the home directory — acceptable for now given this codebase's existing whole-file-in-memory convention, but worth revisiting if package sizes grow enough to matter.

## Follow-Up

- `import`/`pull`/`run` (spawn) don't call `snapshot.Create` yet, matching ADR 0013's same deferred-wiring stance for the graph itself — a natural next slice once this lands.
- No CLI surface exists yet for inspecting a snapshot directly (e.g. `lineage graph list` shows the `snapshot_id` but there's no `lineage snapshot show <id>` or restore command) — deferred until there's a concrete use for it, rather than building an inspection UI speculatively.
- If package content sizes ever make "read the whole file into memory twice" a real cost, `ComputeDigest` and `Create` could share a single content-read pass instead of each re-reading from disk.
