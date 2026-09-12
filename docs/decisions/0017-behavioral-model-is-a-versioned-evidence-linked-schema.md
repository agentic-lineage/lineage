# 0017 Behavioral Model Is A Versioned, Evidence-Linked Schema

Status: Accepted

Date: 2026-09-12

## Context

#203 gave the compiler a deterministic evidence inventory of a source
workspace, but nothing that turns that evidence into a provider-neutral
description of what the workflow actually does. Without such a model, the
next stage in the pipeline (#104, agent-assisted analysis) has no target
schema to fill in, and #106 (artifact compilation) has no basis for staying
provider-neutral other than convention.

Per ADR 0016, compilation proceeds evidence inventory → behavioral model →
agent-assisted analysis → provider-neutral artifacts → portability and
behavior validation. This ADR is the second stage.

## Decision

1. A new `internal/model` package defines `BehavioralModel`: a versioned,
   provider-neutral representation of one compiled workflow — ordered
   `Step`s, each with typed `Claim`s for inputs, outputs, required skills,
   tools, and references, plus setup needs and validation gates — and a
   list of `Decision`s for unresolved or ambiguous behavior.
2. `BehavioralModel.Schema` follows `packages.Manifest.Schema` and
   `inventory.Inventory.Schema`'s exact versioning convention: an absent
   field defaults to `CurrentSchema`, any other value is rejected outright.
3. `BehavioralModel.SourceInventoryDigest` pins the whole
   `inventory.Inventory` snapshot the model was built from, on top of (not
   instead of) per-citation evidence checks — a signal individual
   `EvidenceRef` comparisons alone cannot give, since those only ever
   compare one cited file at a time.
4. Every field-level assertion is a `Claim` carrying its own `EvidenceRef`s,
   rather than one evidence list shared across a whole step, so which
   evidence supports which specific assertion is never ambiguous — the
   exact gap a later reasoning stage would otherwise have to guess past.
5. No field carries a synthetic identifier where a natural one already
   exists: `Claim.Value` and `SetupNeed.Path` already are unique within
   their scope, so `Ref.Key` reuses them directly. `Gate` is the one
   exception, needing a real `ID` since its only content field is free
   prose.
6. `Decision.Refs` addresses model-level, step-level, field-level, or
   claim-level ambiguity — the same `Ref` type at four granularities —
   rather than forcing every decision to name an exact claim that may not
   exist yet.
7. `Validate(m, inv)` collects every problem it finds, following
   `packages.Validate`'s pattern exactly, rather than failing fast.
8. This package does not resolve ambiguity, emit provider artifacts, or
   execute anything it discovers. It structures and validates evidence
   that already exists.

## Consequences

- #104 has a concrete schema to fill in: resolving a `Decision` means
  producing or updating the `Claim` its `Refs` point at.
- #106 has an explicit, if partial, mapping from model fields to
  `packages.Manifest` concepts (documented in
  `docs/guides/compiling-existing-workspaces.md`), and a clear signal
  (unresolved `Decision`s) for when it must refuse to compile.
- No artifact emission, receiver setup execution, or script execution is
  introduced by this stage.

## Follow-Up

- #106 must implement the model-to-manifest mapping as code, not just
  documentation.
- #104 must resolve or explicitly carry forward every `Decision` before
  compilation is allowed to proceed — silently dropping one would violate
  ADR 0016 rule 3.
- `Gate` semantics (what actually runs a validation gate) are deferred to
  #109/#113.
