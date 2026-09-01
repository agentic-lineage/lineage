# 0011 Export/Import Treats Archives As Untrusted Input, With No Signing In V1

Status: Accepted

Date: 2026-08-19

Updated: 2026-08-26 — registry distribution now verifies publisher identity;
local archive exchange remains unsigned.

## Context

ADR 0005 gave packages a verifiable identity (`schema`, content digest) but explicitly deferred the question it exists to answer: once `lineage package export` and `lineage package import` let package content leave the machine that authored it, what does a receiver actually get to trust about an archive before its content starts materializing into their project? Two shortcuts were available and rejected: trusting an archive because it parses as valid tar.gz (an archive is just bytes from an arbitrary source, no different in trust level than a hand-edited manifest), and skipping re-validation on import because the package "already passed validation" at export time (the archive could have been edited, corrupted, or substituted in transit).

## Decision

- `Export` refuses to run at all against a package that fails `Validate` — a secret finding, a traversing entrypoint, or a declared-but-missing export blocks the archive from being created in the first place, rather than shipping a known-bad package and hoping the receiver catches it.
- `Import` treats every archive as fully untrusted, regardless of source: each entry path is checked with `SafeJoin` before anything is written (ADR 0010's package-controlled-input rule applies to archive entries the same as manifest fields), and the fully extracted content is run through the complete `Validate` pass again — the same checks as `lineage package validate` — before it is kept. A previously-valid export that fails validation on import (edited, corrupted, or tampered with) is discarded, not installed.
- Export produces deterministic bytes (sorted file order, normalized permissions, a fixed archive timestamp) so identical package content always produces an identical archive, and therefore an identical digest — this makes "did import reconstruct exactly what export produced" a checkable fact rather than an assumption.
- Local archive exchange has no signing and does not establish sender identity.
  The digest proves *what* was imported is byte-identical to what an export
  produced, not *who* produced it. Registry distribution adds verified GitHub
  publisher identity and immutable name/version ownership (ADR 0012), while
  retaining the same import-time validation and digest verification.

## Consequences

A receiver can trust that anything materialized from an imported package passed the same safety checks as a locally-authored one — trust flows from re-validation, not from the archive's origin. This means import is slower than a naive extract-and-go (a full `Validate` pass runs twice: once implicitly via `Export`, once explicitly via `Import`), which is an intentional trade against silently trusting transit-modified content. Registry packages additionally identify the publisher, but they are not artifact-signed; local archive exchange remains re-validation-only.

## Follow-Up

Artifact signing remains out of scope. Revisit it only if verified registry
identity plus re-validation is insufficient for real receivers. The state this
decision produces (imported packages under `user/packages/`, and the archives
themselves) is covered by ADR 0015's `.lineage` inventory.
