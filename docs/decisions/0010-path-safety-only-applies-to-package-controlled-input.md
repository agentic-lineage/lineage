# 0010 Path Safety Only Applies To Package-Controlled Input, Not User-Typed Paths

Status: Accepted

Date: 2026-08-19

## Context

Adding traversal protection for manifest-derived paths and archive entries raised an easy-to-get-wrong question: should the same guard also apply to paths the user types directly at the CLI, like `lineage enable ../sibling-package`? Blanket-rejecting `..` everywhere would break a legitimate use case (referencing a package outside the current project, e.g. in a monorepo) to guard against a threat that isn't there — the user typing their own CLI arguments is trusted first-party intent, not untrusted package content.

## Decision

`internal/packages.SafeJoin` is scoped specifically to paths that originate from *package-controlled* input: manifest fields (`entrypoints.claude`/`entrypoints.codex`) and archive entries during import. It is never applied to a path the user supplies directly as a CLI argument (`lineage enable <ref>`, `lineage package validate <path>`) — those keep resolving exactly as typed, `..` included.

## Consequences

A new manifest field whose value becomes a filesystem path must be run through `SafeJoin` before use — that's the rule to apply, not "validate all paths." Conversely, CLI argument handling should never gain a traversal check on the theory that it's "safer" — doing so would silently break legitimate relative references the user is entitled to make. The dividing line is *who controls the string*, not *whether it looks like a path*.

## Follow-Up

`lineage package import <archive>` applies this rule to archive entry paths
before anything is written to disk, the same as manifest entrypoints.
