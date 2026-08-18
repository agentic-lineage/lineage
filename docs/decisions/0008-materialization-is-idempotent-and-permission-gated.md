# 0008 Materialization Is Idempotent, Reversible, And Permission-Gated

Status: Accepted

Date: 2026-08-19

## Context

Making `lineage run` actually stage a package's content for a provider (skills into `.claude/skills/`, a generated section in `CLAUDE.md`) is the core of what makes "enable" mean something. That also means `lineage run` writes to the receiver's project for the first time. Two failure modes needed a decision up front: repeated or shrinking runs silently drifting (duplicated entries, orphaned files from a disabled package), and materialization happening as an unannounced side effect of what looks like "just launching the agent."

## Decision

- Materialization tracks exactly what it wrote per provider in `.lineage/materialized-<provider>.json`. Every `Apply` call reconciles against that record — the desired set for the current package list, not a diff against whatever happens to be on disk — so re-running never duplicates and a package that's no longer enabled has its output removed.
- `lineage run` computes whether materializing would change anything (`NeedsApproval`) and, if so, shows the same summary `--dry-run` would and asks for confirmation before writing. Approval is remembered implicitly: an unchanged package set doesn't re-prompt. `--yes`/`-y` skips the prompt for scripts. Declining aborts the entire run, including launching the provider — there is no silent "launch without materializing" fallback.

## Consequences

A receiver never discovers unexpected files in their project from `lineage run` without having seen what was about to happen first. Provider adapters don't need their own approval or cleanup logic — anything built as an `internal/provider.Provider` entry inherits both behaviors from `internal/materialize` for free. The cost is one interactive prompt on first use per project/provider pair, and CLI automation must remember `--yes`.

## Follow-Up

`lineage disable <ref>` (Phase 5 of the distribution plan) can reuse `Apply` directly with a shortened package list — the reconciliation behavior already supports it, no new removal API needed. Setup actions beyond materialization (declared tracker/template files, permission-gated receiver setup — Phase 2 territory) should gate through the same confirmation pattern rather than inventing a second one.
