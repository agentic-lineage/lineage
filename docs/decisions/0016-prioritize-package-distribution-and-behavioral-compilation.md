# 0016 Prioritize Package Distribution And Behavioral Compilation

Status: Accepted

Date: 2026-08-31

## Context

Lineage explored a broader enterprise context-runtime thesis: inherited
organization and team knowledge, provider-side context reuse, and branchable
context state. Those ideas may be valuable, but they are not the shipped
product surface or the next risk to resolve.

The public runtime now supports a complete package loop. What remains unproven
is whether real workflows can be compiled without losing their important
behavior, whether receivers can assess package risk clearly, and whether an
independent package ecosystem will use the loop.

## Decision

1. The active product focus is local, provider-neutral package distribution,
   receiver trust, and behavioral compilation of existing agent workspaces.
2. Compilation proceeds in an explicit order: evidence inventory → behavioral
   model → agent-assisted analysis → provider-neutral artifacts → portability
   and behavior validation.
3. Source analysis is read-only. It must not execute workspace scripts or
   silently turn uncertain evidence into asserted behavior.
4. Provider adapters may project a package into a provider's native surface,
   but they do not change the core package format or make Lineage an agent
   provider.
5. Enterprise context-runtime ideas remain research. Lineage will not claim
   token savings, cache reuse, or organization/team context inheritance until
   those hypotheses are separately validated and deliberately scoped.

## Consequences

- The roadmap prioritizes package fidelity, trust, and real adoption over new
  context infrastructure.
- Open adapter pull requests are documented as work under review, never as
  released support.
- The evidence inventory introduced in #203 is an internal contract for later
  compilation stages, not a public CLI command or a semantic understanding
  engine.

## Follow-Up

- Keep the delivery sequence and linked issues current in `ROADMAP.md`.
- Revisit context-runtime work only when real authors or receivers expose a
  package-runtime problem that a new layer can solve.
