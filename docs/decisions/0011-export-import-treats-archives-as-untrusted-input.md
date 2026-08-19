# 0011 Export/Import Treats Archives As Untrusted Input, With No Signing In V1

Status: Accepted

Date: 2026-08-19

## Context

ADR 0005 gave packages a verifiable identity (`schema`, content digest) but explicitly deferred the question it exists to answer: once `lineage package export` and `lineage package import` let package content leave the machine that authored it, what does a receiver actually get to trust about an archive before its content starts materializing into their project? Two shortcuts were available and rejected: trusting an archive because it parses as valid tar.gz (an archive is just bytes from an arbitrary source, no different in trust level than a hand-edited manifest), and skipping re-validation on import because the package "already passed validation" at export time (the archive could have been edited, corrupted, or substituted in transit).

## Decision

- `Export` refuses to run at all against a package that fails `Validate` — a secret finding, a traversing entrypoint, or a declared-but-missing export blocks the archive from being created in the first place, rather than shipping a known-bad package and hoping the receiver catches it.
- `Import` treats every archive as fully untrusted, regardless of source: each entry path is checked with `SafeJoin` before anything is written (ADR 0010's package-controlled-input rule applies to archive entries the same as manifest fields), and the fully extracted content is run through the complete `Validate` pass again — the same checks as `lineage package validate` — before it is kept. A previously-valid export that fails validation on import (edited, corrupted, or tampered with) is discarded, not installed.
- Export produces deterministic bytes (sorted file order, normalized permissions, a fixed archive timestamp) so identical package content always produces an identical archive, and therefore an identical digest — this makes "did import reconstruct exactly what export produced" a checkable fact rather than an assumption.
- V1 has no signing, no publisher identity, and no distribution registry. The digest proves *what* was imported is byte-identical to what an export produced; it does not prove *who* produced it. Import's authenticity story in v1 is entirely "we independently re-validated the content," not "we verified who sent it."

## Consequences

A receiver can trust that anything materialized from an imported package passed the same safety checks as a locally-authored one — trust flows from re-validation, not from the archive's origin. This means import is slower than a naive extract-and-go (a full `Validate` pass runs twice: once implicitly via `Export`, once explicitly via `Import`), which is an intentional trade against silently trusting transit-modified content. It also means v1 has no answer yet for "is this package really from who it claims to be" — that's a distinct, harder problem than "is this package's content safe," and conflating them would have blocked shipping export/import on an unscoped trust-and-identity design.

## Follow-Up

Publisher identity and archive signing are out of scope for v1 and should get their own decision record if/when a distribution registry or multi-party sharing model is designed — re-validation-only trust does not scale to "I got this from someone I don't know." The state this decision produces (imported packages under `user/packages/`, and the archives themselves) is exactly the kind of thing issue #59 (defining the `.lineage` artifact) should account for when it enumerates what's authoritative vs. regenerable.
