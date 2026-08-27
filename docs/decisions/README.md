# Decision Log

This folder records important product and technical decisions for Lineage.

Use this log when a decision will be expensive to reverse, affects contributors, shapes package semantics, or clarifies why the project chose one path over another.

## Format

Decision records use short ADR-style files:

```text
NNNN-short-title.md
```

Each record should include:

- Status
- Context
- Decision
- Consequences
- Follow-up

## Status Values

- `Proposed`: under discussion.
- `Accepted`: current project direction.
- `Superseded`: replaced by a newer decision.

## Decisions

- [0001 Use Go For The Local Runtime](0001-use-go-for-local-runtime.md)
- [0002 Keep Core Provider-Agnostic](0002-keep-core-provider-agnostic.md)
- [0003 Use Goal-Based Milestones](0003-use-goal-based-milestones.md)
- [0004 Require Issue Links And Test Plans For PRs](0004-require-issue-links-and-test-plans-for-prs.md)
- [0005 Manifest Is Authoritative, With Schema Versioning And A Content Digest](0005-manifest-is-authoritative-with-schema-and-digest.md)
- [0006 Capabilities Are Declared, Not Enforced, In V1](0006-capabilities-are-declared-not-enforced.md)
- [0007 Providers Are A Single Registry Entry](0007-providers-are-a-single-registry-entry.md)
- [0008 Materialization Is Idempotent, Reversible, And Permission-Gated](0008-materialization-is-idempotent-and-permission-gated.md)
- [0009 Secret Scanning Is A Documented Allow/Denylist, Not A General Engine](0009-secret-scanning-is-a-documented-list-not-an-engine.md)
- [0010 Path Safety Only Applies To Package-Controlled Input, Not User-Typed Paths](0010-path-safety-only-applies-to-package-controlled-input.md)
- [0011 Export/Import Treats Archives As Untrusted Input, With No Signing In V1](0011-export-import-treats-archives-as-untrusted-input.md)
- [0012 V1 Distribution Contract And Receiver Activation](0012-v1-distribution-contract-and-receiver-activation.md)
- [0013 Local Lineage Graph Is An Append-Only Descendant Log](0013-local-lineage-graph-is-an-append-only-descendant-log.md)
- [0014 Content-Addressed Snapshot Store Separates Objects From Manifests](0014-content-addressed-snapshot-store-separates-objects-from-manifests.md)
