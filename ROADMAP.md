# Roadmap

Lineage's current product is the local, provider-neutral package runtime:
package a working agent environment, let another person inspect it, enable it
locally with explicit approval, and run it through their own coding agent.

This roadmap records shipped work and the ordered work that follows it. Issue
and pull-request state remains the source of truth for individual tasks.

## Done

- Package creation, validation, deterministic export, untrusted import, and
  digest verification.
- Registry publishing and pulling through the Lineage website API, GitHub
  publisher identity verification, immutable package versions, and `lineage
  add` as the one-command receiver path.
- Receiver controls: inspect-before-enable, permission-gated setup and
  materialization, idempotent cleanup, dry runs, and `lineage doctor`.
- Claude and Codex materialization, workflow-scoped runs, provider shims, and
  provider-neutral launch planning.
- The August consolidation pass: package-name validation, digest and secret
  scanning fixes, atomic writes, safe archive extraction, and confirmation-flow
  fixes. The remaining accepted concurrency limitation stays tracked in #170.
- Windows CI and the checked-in author-to-receiver end-to-end fixture (#174,
  #207, #211).
- The `.lineage` container specification, lineage graph, and content-addressed
  snapshots (ADR 0013–0015).
- A high-confidence Google API-key pattern in package secret scanning (#229).
- Read-only source-workspace evidence inventory: deterministic classification,
  content digests, and literal Markdown citation edges (#203).

## Now

1. **Integrate the current contribution queue without weakening package
   semantics.** Review, rebase, test, and merge provider and diagnostics work
   only when it remains behind the provider boundary. Open pull requests are
   not release documentation:
   - #215: Auggie adapter and provider-owned skill rendering. Before release,
     align receiver documentation with Auggie's Node.js 22+ requirement and
     make existing `SKILL.md` frontmatter handling robust to CRLF line endings.
   - #210, #220–#222: Cursor, Aider, Cline, and Windsurf adapters.
   - #219: extend `lineage doctor` using the `.lineage` container contract.

2. **Turn existing workspaces into reviewable packages.** #203 completed the
   evidence-inventory stage. The next sequence is deliberately constrained:
   behavioral model (#103), agent-assisted analysis (#104), provider-neutral
   artifact compilation (#106), then portability and behavior validation
   (#109, #113). The compiler must not execute source scripts or silently
   invent missing behavior.

3. **Close receiver trust and lifecycle gaps before broader distribution.**
   Exact-version pinning is the current rollback path (#122). Define registry
   trust states and version yanking (#123, #135), then build the smallest
   useful safety policy and reporting surfaces (#127–#136).

4. **Prove the loop with real packages and receivers.** Prioritize a small
   package ecosystem, independent authors, receiver feedback, and contributor
   adoption over adding speculative runtime layers.

5. **Keep public surfaces synchronized.** When released receiver behavior
   changes, update the README, guides, bootstrap prompt, safety model, website,
   Wiki, Discussions, `llms.txt`, and package-page copy together.

## Later

- Additional provider adapters after the active queue has been integrated and
  the adapter contract has held up in real use.
- Stronger runtime capability enforcement if receiver evidence shows that
  declared capability metadata is insufficient.
- Enterprise context reuse, organization/team inheritance, provider caching,
  and persistent context checkpoints. These are research directions, not
  current Lineage commitments; see ADR 0016.
