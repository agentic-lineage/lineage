---
name: lineage-go-runtime-guardrails
description: Use when writing or reviewing Lineage Go runtime code, package loading, manifests, object storage, materialization, provider registries, shims, or tests. Apply normal Go/backend engineering skill first, then enforce these Lineage-specific constraints.
---

# Lineage Go Runtime Guardrails

This is a Lineage-specific overlay for high-quality Go work. Use broad Go/backend expertise for implementation details; use this skill to keep the code aligned with what Lineage is trying to be.

## Product Boundary

Lineage is a local distribution layer for agent environments. It packages and activates local agent context; it is not a workflow engine, marketplace, cloud service, billing system, vector database, or replacement for Claude/Codex/Cursor/etc.

## Runtime Rules

- Keep core runtime code provider-neutral. Provider-specific behavior belongs behind explicit adapter/registry/shim boundaries.
- Treat packages, manifests, archives, and setup declarations as untrusted input.
- Prefer immutable, deterministic state: stable ordering, normalized paths, content hashes, and reproducible manifests.
- Make operations idempotent. Running the same command twice should not duplicate entries, corrupt files, or drift silently.
- Preserve resumability: partial local work should either be recoverable or fail before unsafe writes happen.
- Avoid package behavior that depends on wall-clock time, machine-specific absolute paths, or hidden global state unless the value is clearly local metadata.
- Keep errors actionable and safe to print. Never include secret values in error output.

## Go Quality Bar

- Use simple packages and clear ownership boundaries before adding abstractions.
- Prefer standard library APIs for filesystem, hashing, archives, and path handling unless a dependency clearly improves safety.
- Keep command logic thin; put testable behavior in `internal/`.
- Sort discovered names before returning or printing them.
- Use table tests for format, validation, path-safety, and idempotency behavior.
- Add regression tests for every bug fix that touches package discovery, config resolution, materialization, launch planning, or safety checks.

## Required Verification

Run:

```bash
go test ./...
```

For CLI-facing behavior, verify output and failure modes in tests. For package/materialization behavior, test repeated runs and unsafe input.
