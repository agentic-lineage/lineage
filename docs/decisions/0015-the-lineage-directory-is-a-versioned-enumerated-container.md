# 0015 The `.lineage` Directory Is A Versioned, Enumerated Container With A Gitignore Default

Status: Accepted

Date: 2026-08-24

## Context

`.lineage` has grown into two distinct on-disk artifacts — project-level `<project>/.lineage/` and user-level `~/.lineage/` (or `$LINEAGE_HOME`) — accreted one field at a time as separate phases landed (issue #59). Each piece has reasonable behavior in isolation: materialization state is idempotent per ADR 0008, the manifest format is versioned per ADR 0005, the local lineage graph is an append-only log per ADR 0013, the snapshot store is content-addressed per ADR 0014. But the container directories themselves had no equivalent spec — no schema field on the state files most of them wrote, no stated commit/gitignore policy for a receiver's project-level `.lineage/`, and no single doc enumerating what's safe to delete versus what's authoritative.

This is deliberately scoped to the container and its state files, not the formats already decided: ADR 0005 (manifest), ADR 0008 (materialization idempotency), ADR 0013 (graph shape), and ADR 0014 (snapshot addressing) all stay exactly as decided.

## Decision

### 1. Enumeration

**Project-level `<project>/.lineage/`** (one per project that has ever run `lineage enable`):

| Path | Written by | Format |
|---|---|---|
| `config.yaml` | `internal/config` | YAML: workspace, enabled packages, provider preferences/overrides |
| `materialized-<provider>.json` | `internal/materialize` | JSON: one file per provider that has ever materialized, tracking exactly what was staged |
| `graph.json` | `internal/graph` | JSON array: append-only log of "this project descended from that package" events (ADR 0013) |

**User-level `~/.lineage/`**:

| Path | Written by | Format |
|---|---|---|
| `user/packages/` | `internal/config`/`lineage init user` | Directories: imported/authored packages |
| `workspaces/<name>/packages/` | `internal/config`/`lineage init workspace` | Directories: workspace-scoped packages |
| `bin/` | `internal/shim` | Generated provider shim scripts/batch files |
| `objects/<hash[:2]>/<hash[2:]>` | `internal/snapshot` | Content-addressed file objects (ADR 0014) |
| `snapshots/<hash[:2]>/<hash[2:]>` | `internal/snapshot` | Content-addressed snapshot manifests, schema-versioned (ADR 0014) |
| `github_token` | `internal/auth` | Plain-text bearer token, `0600`, written by `lineage login` |

### 2. Schema versioning

`config.yaml` and `materialized-<provider>.json` now carry a `schema` integer field, following the exact convention ADR 0005 set for `lineage.yaml` and ADR 0014 already applied to snapshot manifests: a missing field (every file written before this decision) defaults to `1`; anything else this build doesn't recognize is a hard load error naming both the declared and understood schema, rather than a parser silently misreading a future format.

- `internal/config.ProjectConfig.Schema`, defaulted/validated in `LoadProjectConfig` (`CurrentConfigSchema = 1`).
- `internal/materialize`'s internal `state.Schema`, defaulted/validated in `loadState` (`currentStateSchema = 1`).
- Snapshot manifests already had this (`snapshot.CurrentManifestSchema`, ADR 0014) — no change needed there, just confirming it satisfies this decision.

`graph.json` deliberately does **not** get a schema field in this change. It's currently a bare JSON array, not an object — adding a top-level `schema` key would mean wrapping it in an object, a breaking shape change to a format that shipped days before this decision. Each `Record` inside the array can already gain new optional fields without any wrapper (old records stay valid input for a newer reader), which covers the additive case a schema field mainly exists to survive. If the array's own shape ever needs a breaking change — not just a new optional field on a record — that's the trigger to introduce a wrapping object with its own `schema`, not before.

`bin/` shims and `github_token` aren't structured, evolvable formats — shims are fully regenerated on every `install-shims` run, and the token is a single opaque string — so schema versioning doesn't apply to either.

### 3. Commit / gitignore convention

Project-level `<project>/.lineage/` must never be committed to a receiver's repo. `config.yaml` can carry machine-local provider binary paths (`providers.<name>.binary`), and `materialized-<provider>.json`/`graph.json` are local-environment state, not portable package content — none of it is meant to travel with the project's own source.

`lineage enable` (`internal/cli.enableRef`) now calls `config.EnsureGitignored` after every successful enable — appending a `.lineage/` line to the project's `.gitignore`, creating the file if it doesn't exist. It's a no-op if the file already has either `.lineage/` or a bare `.lineage` entry, so repeated enables don't duplicate the rule. Reinforcing the entry each time means an accidentally removed ignore rule is restored before local provider paths and materialization state become easy to commit. There's no single `lineage init <project>` command today — `lineage enable` is the de facto point a project first gets a `.lineage/` at all, so that's where this lives; documented in `CONTRIBUTING.md` since there's no other natural home for the policy statement.

This mirrors a decision Lineage already made about its own dev-time state: the root `.gitignore` in this repo has ignored `.lineage/` since `chore/simplify-gitignore` (#74), for the same reason — machine-local, regenerable, never meant to be committed. This decision generalizes that same call to every receiver's project.

### 4. Regenerable vs. authoritative

| Entry | Classification | Notes |
|---|---|---|
| `<project>/.lineage/config.yaml` | Regenerable, not silent | Re-run `lineage enable`/set `providers.*` again; losing it forgets what a receiver chose, but nothing is unrecoverable |
| `<project>/.lineage/materialized-<provider>.json` | Regenerable | Deleted safely; the next `run`/`enable` re-derives it exactly from `config.yaml` + package content (ADR 0008) |
| `<project>/.lineage/graph.json` | **Authoritative** | Append-only history; nothing else records when/what a project's state descended from — deleting it loses provenance permanently |
| `~/.lineage/user/packages/` | **Authoritative** | Hand-authored/imported package content; deleting it loses user work |
| `~/.lineage/workspaces/<name>/packages/` | **Authoritative** | Same as above, workspace-scoped |
| `~/.lineage/bin/` | Regenerable | `lineage install-shims` recreates it fully from scratch |
| `~/.lineage/objects/`, `~/.lineage/snapshots/` | **Authoritative in practice** | Conceptually a derived copy of package content, but once the original package source is gone this is the only durable byte-for-byte copy (the entire point of ADR 0014) |
| `~/.lineage/github_token` | Regenerable, sensitive | `lineage login` recreates it; deleting only forces re-authentication — but never share, log, or commit it |

### 5. Out of scope

Per the issue's own framing: the package manifest format (ADR 0005) and materialization idempotency semantics (ADR 0008) are already decided and unchanged here. Content-addressed snapshots (issue #7) and lineage graph metadata (issue #6) were future capabilities when the issue was filed; both landed since as ADR 0014 and ADR 0013 respectively, and this decision enumerates them as already-settled rather than blocking on them.

`lineage doctor`'s existing checks (project config validity, enabled-package resolution, shim PATH placement, provider binary resolution) now transitively cover `config.yaml`'s new schema field for free — `FindProjectConfig`/`LoadProjectConfig` reject an unsupported schema the same way they already reject unparseable YAML. Extending `doctor` to validate `materialized-<provider>.json` schema/staleness, `graph.json` integrity, and snapshot-store referential integrity (every object a manifest references still present) is real, separate work with its own design questions (what's a warning vs. a failure, whether `doctor` should offer to repair anything) — deferred to #200 rather than folded into this container spec.

## Consequences

`.lineage`'s contents are now enumerated in one place instead of being discoverable only by reading five different packages, and every structured state file this build writes has a stated, checkable schema. A receiver's project no longer accidentally ships machine-local provider paths or regenerable cache into their repo by default. The cost is small and one-directional: `config.yaml` and `materialized-<provider>.json` written by this build are one byte larger (a `schema: 1` line) and older files written before this decision keep loading unchanged.

## Follow-Up

- #200 tracks the `lineage doctor` scope extension covering `materialized-<provider>.json` schema/staleness, `graph.json` parseability, and snapshot-store referential integrity, per the "out of scope" note above.
- If `graph.json`'s array shape ever needs a breaking change, introduce a wrapping object with its own `schema` field at that point, per the schema-versioning decision above.
- A future `lineage clean`-style command (regenerable-only cleanup) can use the regenerable/authoritative table above directly as its spec.
