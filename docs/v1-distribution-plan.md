# Path to v1: an actual agent-environment distribution layer

**Scope decision:** this plan targets what the code and README already describe Lineage as — a way to package, share, inspect, and enable a local agent environment (skills, workflows, agents, policies) so a receiver can run it with their own `claude`/`codex`. It deliberately does not chase PRD1/PRD2's larger "enterprise context runtime" ambitions (token-reuse/caching, provider-side context checkpoints, relevance-based selection) — those depend on this distribution layer existing first, and are a v2+ conversation once this is solid.

**Revision note (2026-08-19):** this plan was revised after an external neutral technical review of the repository. The review's core observation — that the manifest format, package identity, and validation model should be settled *before* the format is widely shared, because changes get expensive after that — reshuffled Phase 2/3 below into a single "lock the format" phase that now comes before export/import. The review's larger strategic ambitions (content-addressed digests, signatures/provenance, lockfiles, dependency resolution, a lineage graph, a registry) are deliberately **not** pulled into v1; see "Deliberately deferred to v2+" at the end. Pulling all of that in now would repeat the mistake the review itself warns against: building distribution machinery before the local artifact model is proven.

## Status

- **Phase 1 (materializer) — in progress.** Core + Claude Code adapter implemented and in review (issue #18, PR #22): `internal/materialize` stages enabled packages' skills where the provider reads them (`.claude/skills/<pkg>-<skill>/`) and maintains a generated section in `CLAUDE.md`, idempotently and reversibly via `.lineage/materialized-<provider>.json`. Wired into `lineage run claude` right before `provider.Launch`, skipped on `--dry-run`. Codex adapter (issue #19) is the remaining piece — same core, `.agents/skills/` + `AGENTS.md`.
- **CLI reliability baseline — landed** (issues #14/#15/#16, PR #17): `lineage enable` project-root resolution, `lineage run ... -- <args>` passthrough, `lineage package init` no longer clobbering an existing manifest.

## Phase 1 — Make enabling a package actually do something (highest priority)

This is the single biggest gap: `lineage run claude` used to differ from running `claude` directly only by two environment variables (`LINEAGE_ACTIVE`, `LINEAGE_PROVIDER` — `internal/provider/provider.go`). Discovered skills/workflows/agents/policies were listed in dry-run output and nowhere else. Until this exists, "distribution" only distributes files sitting next to the agent, not anything the agent is aware of.

- A **materializer** (`internal/materialize`) runs right before `provider.Launch`. For each resolved, enabled package, it stages the package's skills where the target agent actually reads them, plus writes/refreshes a generated section in the provider's own context file (`CLAUDE.md` / `AGENTS.md`) listing active packages, agents, policies, and workflows — so the model has situational awareness even for content it doesn't auto-load.
- Kept behind an explicit adapter boundary (`internal/provider.Adapter`: skills directory + context file, one value per provider) so `internal/materialize`, `internal/runtime`, and `internal/packages` stay provider-neutral per the existing engineering skill's rule.
- Idempotent and reversible: re-running `lineage run` doesn't duplicate entries, and a package that's no longer enabled has what it staged removed — tracked via a per-provider state file (`.lineage/materialized-<provider>.json`) rather than guessed from disk contents.
- Remaining: Codex adapter (`.agents/skills/<pkg>-<skill>/`, `AGENTS.md`) reusing the same core (issue #19).

## Phase 2 — Lock the package format before it spreads

The review's central point: the manifest and package-content model are still informal, and every decision here gets expensive to change once packages are actually being shared (Phase 3). This phase settles the format and turns the README's/`SECURITY.md`'s documented-but-uncoded safety principles into actual checks, before export/import exists to move that content between machines.

**Manifest is authoritative, not a suggestion alongside the filesystem.**
- Add `schema: 1` to `lineage.yaml`, distinct from the package's own `version`. `schema` describes how to interpret the manifest; `version` describes the package. Without this split, any future manifest change has no clean way to signal "old parser can't read this."
- Decide and document how `exports.agents` / `exports.workflows` relate to what's actually discovered on disk. Recommendation: **manifest is authoritative, filesystem is payload** — declared-but-missing entries are a validation error; undeclared-but-present entries are allowed locally but excluded from what `export` ships and what `enable` advertises. This is what makes validation, hashing, and export well-defined instead of "whatever `os.ReadDir` happens to find."
- `requires.skills`: validate at `enable` time (and again at `run` time, since a dependency could be removed later) that every declared required skill is actually discoverable — either bundled in the same package or provided by another currently-enabled package. Fail with a clear error naming the missing skill.
- `entrypoints`: use `entrypoints.claude` / `entrypoints.codex` to decide what a package actually hands to that provider (a specific script/prompt/config path) instead of ignoring the field, now that Phase 1's materializer exists to act on it.

**Package identity.**
- Add a content digest (`sha256` over the manifest-declared file set, in deterministic order) computed at `Discover` time and surfaced in `--dry-run` / `lineage package validate` output. This does not require export/import to exist first — it only requires walking the same files `Discover` already reads, deterministically. Landing this now (rather than bolted onto Phase 3's export) means a logical ref like `security-review@1.4.2` can be paired with "and this is exactly what that resolved to," which is the difference between a version number and a verifiable identity.
- Package identity conflicts: two enabled packages silently exporting the same skill name currently has no conflict signal. Add a check at `ResolveEnabled` time — same skill/agent/policy name from two different enabled packages should at minimum warn rather than silently letting the second one shadow the first.

**Safety checks, made real instead of documented.**
- **Path traversal protection**: `internal/packages` currently does no validation on manifest-derived paths. Add a `safepath` helper used everywhere a package-supplied relative path gets joined onto a filesystem location (this is exactly what Phase 3's import will need too), and unit-test it against `../../etc/passwd`-style payloads.
- **Symlink rejection**: a distributable package should not be able to smuggle a reference to `~/.ssh` or similar. `internal/materialize`'s Claude adapter already refuses to follow symlinks when staging skill content (issue #18) — extend the same rule to manifest/content resolution generally, and to Phase 3's import path once it exists.
- **Secret/credential scanning**: a lightweight pre-enable scan (filename patterns like `.env`, `*.pem`, common credential filenames, plus a small set of high-confidence content patterns like `-----BEGIN PRIVATE KEY-----` or AWS-key-shaped strings) that warns or blocks. Not a full secret-scanning engine for v1 — a documented, testable allowlist/denylist is enough to make the existing "safety principles" true rather than aspirational.
- **Lightweight capability declarations**: an optional `capabilities:` block in the manifest (`filesystem.read`, `network`, at minimum) that's purely *declarative* for v1 — no enforcement engine, just something `lineage package validate` and the enable-time plan can print so a receiver sees "this package declares it wants network access to X" before enabling. Full policy enforcement (the review's "execution security" layer) is a real v2+ project of its own; declaring the field now avoids a breaking manifest change later when enforcement does get built.
- **Permission-gated setup**: now that Phase 1's materializer writes files into the receiver's project, that write step is the thing that needs an explicit confirmation/permission gate — surface what will be created/changed before doing it, similar in spirit to how `--dry-run` already previews the launch plan.

**Tie it together with a real command.**
- `lineage package validate <package-path>`: runs the manifest schema check, the manifest/filesystem authority check, the dependency check, path/symlink/secret checks, and prints the resolved digest and declared capabilities — without enabling anything. Useful standalone and as the Phase 3 pre-export gate. This is the single highest-leverage feature in this phase: it forces every decision above to actually be decided, and gives receivers something concrete to run before trusting a package.

## Phase 3 — Make packages actually *distributable*, not just locally referenceable

Right now a "package" only ever exists as a local directory reachable by relative path, a name under `~/.lineage/user/packages`, or a name under a workspace. There is no way to hand someone a package as a single artifact — this is the core of "distribution" and the biggest literal gap between the product's name/README and its capability. It comes after Phase 2 specifically because export is the moment untrusted package content starts moving between machines, and the format/validation it relies on needs to already be settled.

- `lineage package export <path> [-o file.tgz]`: produce a deterministic, reproducible archive (sorted file order, normalized permissions/timestamps so two exports of identical content hash identically — and identically to Phase 2's content digest) containing the manifest and native asset directories, after running `lineage package validate` and refusing to export if it fails.
- `lineage package import <file.tgz> [--as name]`: unpack into a target packages directory (user/workspace/project), re-running the same path-traversal, symlink, and secret checks against the archive contents before writing anything to disk — an archive is untrusted input the same way a manifest is.
- Exact round-trip test: export → import → discovered `Package` struct is byte-identical, and its digest matches the pre-export digest.

## Phase 4 — Minimal workflow execution

Currently a "workflow" is indistinguishable from a skill at runtime — both are just a name discovered by the presence of a marker file (`SKILL.md` vs `WORKFLOW.md`). Nothing executes a workflow's steps, checkpoints, or validation gates. For v1, this doesn't need to be the full provider-adapted execution model from PRD1 v0.3 — a minimal version that's still genuinely useful:
- Parse `WORKFLOW.md` frontmatter (or a co-located `workflow.yaml`) for an ordered step list referencing skill names within the same package.
- `lineage workflow run <workflow-name>` materializes (Phase 1) the referenced skills in order and hands off to the provider with a generated prompt/instruction file describing the sequence — essentially "compile the workflow into the same materialization Phase 1 already does for ad hoc enabled skills, but ordered and scoped to just this workflow's dependencies."
- This can ship after Phases 1–3; it's valuable but not blocking for "can I distribute an agent environment."

## Phase 5 — CLI completeness and cross-platform correctness

Rounding out the CLI surface to what a real distribution workflow needs day to day:
- `lineage list` (enabled packages in the current project), `lineage disable <ref>` (uses Phase 1's materializer with a shortened package set — `Apply` already supports this, no new removal API needed), `lineage inspect <ref>` (show manifest + discovered contents + Phase 2's digest/capabilities without enabling), `lineage doctor` (sanity-check config, shim installation, PATH ordering, provider binary resolution).
- Windows shims: `internal/shim/shim.go` only ever writes POSIX `sh` scripts. `TestIsShimPath` already skips on Windows ("path prefix semantics differ"), which is a signal this was never exercised there. Add `.cmd` (or PowerShell) shim generation behind a `runtime.GOOS` check before claiming cross-platform support anywhere in the docs.
- `findRealBinary` (`internal/provider/provider.go`) silently returns whatever's first on `PATH`; add a `lineage doctor`-surfaced warning when more than one candidate binary exists, since that ambiguity is exactly the kind of thing that's invisible until it picks the wrong one.

## Phase 6 — Test and CI hardening

- **Fix the build's network dependency for anyone in a restricted environment**: `go.sum`/`go.mod` currently require fetching `gopkg.in/yaml.v3` from `proxy.golang.org`/`gopkg.in`, both of which are blocked in at least this review environment. Either `go mod vendor` the one dependency or document the allowlist requirement in `CONTRIBUTING.md`/CI config.
- Work through `docs/hypothesis-test-plan.md`'s "Needed Tests" list as each phase lands: path traversal rejection, secret detection, symlink rejection (Phase 2), export/import round trip and digest stability (Phase 3), repeated enablement/setup idempotency (Phase 1, already covered), cross-platform path behavior (Phase 5).
- Keep following house style throughout: deterministic output (sort names, normalize paths), package contents treated as untrusted input, provider-specific logic confined to `internal/provider`/`internal/shim`, and a test added for every discovery/config/launch-planning/safety change (per `.agents/skills/lineage-go-engineering/SKILL.md` and `CONTRIBUTING.md`).

## Suggested order of attack

Phase 1 unlocks everything else being meaningful, so it comes first even though it's the most involved (in progress — Claude adapter in review, Codex adapter next). Phase 2 comes before Phase 3 for the same reason the review emphasizes: format and identity decisions are cheap now and expensive once packages are actually moving between machines via export/import. Phase 4 and 5 can happen in parallel with each other once 1–3 are stable, and Phase 6 is continuous rather than a discrete step — each earlier phase ships its own tests as it lands, not as a separate cleanup pass at the end.

## Deliberately deferred to v2+

The external review's strategic framing — that Lineage's strongest long-term position is as an artifact/provenance/supply-chain layer, not just a package manager — is a reasonable direction, but nearly all of the machinery it implies is explicitly **not** v1 work. Building it now would mean designing signatures, trust, and a lockfile against a package format and validation model that hasn't shipped yet. Deferred, roughly in the order they'd become relevant once Phases 1–3 are solid:

- **Lockfile** (`lineage.lock` recording exact resolved package versions + digests) and real dependency resolution (version constraints, transitive deps, conflicts) — only meaningful once packages have more than one dependency in practice.
- **Signatures and provenance** (publisher identity, source repo/commit, signed digests) and a **trust model** (`lineage trust add/list`) — this is what turns Phase 2's content digest from "tamper-evident" into "tamper-evident and attributable," but needs real users publishing real packages first to design against.
- **`lineage graph` / `lineage diff` / `lineage explain`** — structured lineage as a first-class graph (workspace → package → skill → workflow → provider) instead of the current formatted-text dry-run output. High-value, genuinely differentiating, but a UI/data-model layer on top of a format that needs to be stable first.
- **Full capability enforcement** — Phase 2 adds *declared* capabilities (visible, not enforced). Actually gating agent behavior on them (network egress control, filesystem scoping) is a substantial sandboxing project in its own right and shouldn't block distribution shipping.
- **A registry/marketplace** — explicitly sequenced last by the review too: local package → validated artifact → deterministic export → digest → signature/provenance → *then* registry, not the other way around.
