# 0006 Capabilities Are Declared, Not Enforced, In V1

Status: Accepted

Date: 2026-08-19

## Context

A package with no executable code and no captured secrets can still instruct an agent to read files outside the workspace or reach an unexpected network endpoint. Actually gating that behavior — network egress control, filesystem scoping — is a real sandboxing project, not something to build incidentally while shipping the distribution layer. But if the manifest format ships without any place to declare intent, adding one later is a breaking change to every existing package.

## Decision

Add an optional `capabilities` block to the manifest (`filesystem.read`, `network`) that is purely declarative. `lineage package validate` and `--dry-run` print what a package declares wanting. Nothing in this version of Lineage enforces, blocks, or warns based on those values beyond printing them.

## Consequences

Receivers get visibility ("this package declares it wants network access to X") before enabling, which is real safety value on its own. It also means a `capabilities` block that looks unenforced is expected, not a bug — a contributor should not assume declaring a capability does anything beyond making it visible. Building real enforcement later is additive (interpret an existing field) rather than a manifest schema break.

## Follow-Up

Full capability enforcement is out of scope until there's a concrete sandboxing design to enforce against. When that work starts, it should treat this field as the existing contract to honor, not redesign.
