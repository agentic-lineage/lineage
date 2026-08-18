# 0007 Providers Are A Single Registry Entry

Status: Accepted

Date: 2026-08-19

## Context

0002 established that the core stays provider-agnostic and provider-specific behavior belongs behind explicit boundaries. In practice, `claude` and `codex` had drifted into two places that special-cased their names as string literals: `internal/runtime.BuildPlan`'s provider check and the CLI's usage text. The actual provider-specific behavior (binary resolution, launch, materialization target) was already generic — only the "which providers exist" question was hardcoded, and hardcoded twice.

## Decision

`internal/provider` owns a single registry: `[]Provider{Name, SkillsDir, ContextFile}`, with `Known()`, `Get()`, `IsKnown()` as the only way anything else learns a provider exists. `internal/runtime`, `internal/materialize`, and the CLI's usage/error text all consult this registry instead of naming `claude`/`codex` directly.

## Consequences

Adding a third provider is a one-entry change in one file. CLI help text and error messages can never drift from what's actually registered, since they're generated from the same source. This is the concrete mechanism behind 0002's principle for the provider boundary specifically — 0002 stays the higher-level statement, this record is what "provider-agnostic" means operationally for `internal/provider`.

## Follow-Up

When a real third provider is added, this decision is the thing to validate: if adding it touches more than `internal/provider`'s registry entry (plus a provider-specific adapter implementation if behavior genuinely diverges), the boundary has leaked and should be tightened again.
