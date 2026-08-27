# 0013 Local Lineage Graph Is An Append-Only Descendant Log

Status: Accepted

Date: 2026-08-24

## Context

Issue #6 asks Lineage to explain, locally, where a project's environment came from once a package has been imported, enabled, or spawned into it. There's no existing identifier to build on beyond package identity itself: no snapshot/content-address store yet (issue #7 is a sibling, not a dependency), no workspace ID (`config.ProjectConfig.Workspace` is a human-chosen name), and no session ID anywhere in the runtime. The record format has to be useful today without those, and forward-compatible once they exist.

The issue also explicitly warns against storing private transcript content, and asks that provider-specific metadata stay optional and clearly separated from the core record.

## Decision

- A local lineage graph record (`internal/graph.Record`) captures three things: a `Parent` (the package the environment descended from, identified the same way package identity is identified everywhere else in Lineage — `name`, `version`, and the sha256 `Digest` from `packages.ComputeDigest`, per ADR 0005), a `Descendant` (the local environment that came from it — currently just the workspace name), and an `Event` string naming what caused the record. `SnapshotID` and `SessionID` fields exist on `ParentRef`/`DescendantRef`; `SnapshotID` is populated from `internal/snapshot.Create`'s returned object ID (ADR 0014, implemented alongside this same change), and `SessionID` stays reserved and empty until a session concept exists.
- Provider-specific metadata is a separate `map[string]string` field on the record, never merged into `Parent`/`Descendant`. There is no field anywhere for transcript or message content — the constraint is satisfied by the schema not having a place to put it, the same "declare, don't enforce" posture as ADR 0006, rather than a runtime content scanner.
- Records are stored as JSON (internal machine state, not hand-edited — same convention as `internal/materialize`'s state files) in an append-only array at `<project>/.lineage/graph.json`. Unlike `internal/materialize`'s reconcile-to-desired-state model, this is a log of events over time, not a snapshot of current desired state, so records are only ever appended, never rewritten or pruned.
- `internal/graph.Append` assigns each record an opaque ID (16 random bytes, hex, via `crypto/rand`) and a `CreatedAt` timestamp if the caller left them unset, then appends and saves. `Load` reads all records; `ForPackage` filters by parent package name.
- Wiring is scoped to one flow for this issue: `lineage enable` (`internal/cli.enableRef`) appends an `"enable"` record once the enable actually succeeds. `lineage add`/`import`/`pull` and `lineage run` (spawn) are real "descendant" moments too, but wiring all of them in one PR would make this harder to review; that's deferred (see Follow-Up).
- Inspection is a new `lineage graph list [--yaml]` command, reading `.lineage/graph.json` for the current project.

## Consequences

A receiver (or the maintainer) can ask "what package did this project's enabled state come from, and when — and is there a durable, verifiable copy of exactly what that was" for anything enabled after this change, without needing any session concept to exist first. The cost: enabling a package now does a couple of small extra disk operations (a digest recompute plus a full content-addressed snapshot), and until the deferred wiring below lands, `import`/`pull`/`run` don't yet produce descendant records, so the graph is incomplete relative to the issue's full "why" until that follow-up work happens.

## Follow-Up

- Wire `graph.Append` into `lineage add`/`import`/`pull` (`Event: "import"`) and into `lineage run`'s spawn path (`Event: "spawn"`), each as its own small, reviewable change.
- If a session concept is ever introduced for a provider run, populate `DescendantRef.SessionID` from it instead of leaving it empty.
