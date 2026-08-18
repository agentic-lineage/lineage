# 0009 Secret Scanning Is A Documented Allow/Denylist, Not A General Engine

Status: Accepted

Date: 2026-08-19

## Context

The README, `SECURITY.md`, and the `lineage-package-safety` skill all state that Lineage shouldn't make it easy to accidentally distribute credentials, but none of it was backed by code. A full secret-scanning engine (entropy analysis, broad pattern libraries, ML-based detection) is a substantial project with a real false-positive-rate tuning problem, and building one wasn't a prerequisite for making the existing safety principles true rather than aspirational.

## Decision

`internal/packages.ScanForSecrets` checks a small, explicit, documented set of signals: denylisted filenames (`.env`, `.npmrc`, SSH private key names, `.pem`/`.key`/`.pfx`/`.p12`) and a short list of high-confidence content patterns (private key headers, AWS access key ID shape, GitHub token prefixes). Findings report a file path and a human-readable reason only — the matched value itself is never included in a finding, so scan output is always safe to print or log.

## Consequences

The scan is precise (low false-positive rate) rather than exhaustive — it will not catch every possible secret shape, and that's an accepted tradeoff for v1, not an oversight. It's also trivially extensible: adding a new pattern is a one-line change to a reviewable list, not a retrain or a new subsystem. `lineage package validate` and the future export gate (Phase 3) both consume this as one input among several, not as a claim of complete secret safety.

## Follow-Up

If false negatives against real-world packages become a recurring problem, extend the documented list rather than reaching for a general entropy-based scanner as the first response — that changes the false-positive tradeoff this decision deliberately made.
