# 0016 Instruction-Risk Scanning: A Documented Pattern List With A Severity Split

Status: Accepted

Date: 2026-08-23

## Context

`ScanForSecrets` protects against a package carrying a credential. It does nothing about a package whose *instructions* are the risk: a skill or workflow step that tells an agent to ignore its prior instructions, read broadly outside the workspace, exfiltrate secrets over the network, disable its own safety checks, delete files without confirmation, or collect passwords/tokens/session data. Issue #128 asked for a scanner that catches this class of risk at publish, inspect, and enable time, using the same "small, explainable rule set" tradeoff `ScanForSecrets` already made (0009) rather than a general prompt-safety engine or an LLM-based reviewer - both explicitly out of scope.

Two decisions had to be made before any code could be written: which patterns are severe enough to block a package outright versus which should only warn and require confirmation, and whether a package author should be able to annotate a flagged pattern as expected/benign.

## Decision

`internal/packages.ScanForInstructionRisk` matches package instruction content against six documented categories, each with an initial severity:

- **Hard stop:** `exfiltration` (network verb near a credential-shaped noun), `silent_destructive` (a destructive verb paired with "without confirmation/asking/permission"), `credential_collection` (collecting passwords, private keys, or session data).
- **Warning:** `prompt_override` ("ignore previous instructions" and equivalents), `broad_local_read` ("read all files", "scan the home directory").
- **Conditional:** `disable_safety` ("disable safety checks", "do not ask the user") is a warning on its own, escalated to a hard stop only when the *same file* also contains a `silent_destructive`, `credential_collection`, or `exfiltration` match - disable-safety language alone is common in benign contexts (a skill that legitimately skips confirmation for read-only steps).

A package author may attach a justification to a finding, but a justification is never allowed to suppress it - the finding still blocks or warns exactly as it would unannotated. This mirrors 0006's "capabilities are declared, not enforced": self-attestation is a note for a human reviewer, not a control. (Manifest-level annotation storage is not implemented as part of this change - see Follow-Up.)

Scope is the package's instruction-bearing surfaces: `skills/`, `workflows/`, `agents/`, `policies/`, `adapters/`, plus `setup.files[].template`. The last one needed an explicit second pass: it's a string embedded in `lineage.yaml`, not a file any directory walk reaches, and would otherwise be silently missed by a scanner modeled on `contentFiles`. `references/` is excluded - it's declared payload data, not directives.

A symlink under a scanned surface is rejected with an error, matching `discovery.go`/`materialize.go` rather than `ScanForSecrets`'s silent skip. An oversized or unreadable file produces an `unscanned` finding instead of being dropped with no trace, so "found nothing" and "couldn't check" are never the same signal.

`ValidateReport` gained an `InstructionFindings` field and a `BlockingCount()` method (`len(Errors)` plus any block-severity instruction finding); `Passed()` is now defined in terms of `BlockingCount()`. Because `Export` and `Publish` already refuse on `!report.Passed()`, this alone satisfies the publish-pipeline requirement with no changes to either. `enable` and `inspect` previously called only `Discover`, which never ran any scan (secret or instruction) - both now call the new scanner directly: `enable` refuses outright on a blocking finding and requires confirmation for a warning (reusing the existing setup-plan confirm gate rather than a second prompt - see Consequences); `inspect` surfaces findings by file in both its human-readable and `--yaml` output.

## Consequences

A package that was previously silent about this risk class now either blocks at publish/enable or requires a receiver to explicitly see and approve it first. `enable`'s risk-warning and setup-plan confirmations were combined into one prompt rather than two sequential ones: `confirm()` opens a fresh `bufio.Reader` over `stdin` on every call, so calling it twice in one invocation risks the first call's read-ahead silently consuming a second piped answer before the second call ever sees it. This was judged safer to design around than to fix at the `confirm()` level, since changing that helper's signature would touch every one of its call sites (`enable`, `run`, `workflow run`, `add`) for a fix broader than this issue's scope.

This is a practical risk signal, not complete prompt-injection defense: the pattern list is small and literal, with no attempt at obfuscation resistance (Unicode normalization, zero-width-character stripping) in this pass. A sufficiently obfuscated instruction will not be caught, and a clean scan is not a safety guarantee - the same posture `ScanForSecrets` already takes, stated here explicitly rather than left implicit.

## Follow-Up

Deliberately out of scope for this change, tracked separately:

- **Content-integrity re-scanning at materialization time** (#142): today's `materialize.NeedsApproval` diffs only the set of skill-directory names between runs, not content, so a package's instructions can change after enable-time approval and re-materialize on the next `lineage run` without a new scan or confirmation. Fixing this is a broader trust-boundary change to `internal/materialize`'s state format, out of scope here.
- **Manifest-level annotation storage.** The policy above (a justification never suppresses a finding) is decided, but no `lineage.yaml` field or CLI surface exists yet to actually attach one. Add it as an additive schema field when there's a concrete authoring UX to attach it to, not preemptively.
- **Obfuscation resistance** (Unicode normalization, zero-width-character stripping) and a scanner-ruleset version marker on reports, if false negatives against real packages become a recurring problem - extend the documented pattern list first, matching 0009's stated preference for growing the list over reaching for a heavier detection mechanism.
- **External public-surface sync.** `docs/architecture.md`'s trust model now mentions this scan, but the website, GitHub Wiki, and Discussions seed threads listed in `docs/public-docs-sync.md`'s "Public Surfaces To Sync" checklist are outside this repo and have not been updated to describe it.
