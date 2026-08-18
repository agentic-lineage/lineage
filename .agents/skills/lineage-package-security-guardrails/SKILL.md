---
name: lineage-package-security-guardrails
description: Use when changing Lineage package manifests, validation, export/import, archives, setup prompts, file materialization, secret scanning, path handling, or receiver activation. Apply general security-review practice first, then enforce these Lineage package-safety rules.
---

# Lineage Package Security Guardrails

Lineage packages are portable local environments. A receiver should be able to inspect what a package will do before it changes their workspace.

## Threat Model

Assume every package is untrusted until validated. A malicious or careless package may try to:

- include secrets or private local state;
- reference files outside the package;
- write outside the intended workspace;
- smuggle symlinks or path traversal entries;
- hide risky setup actions behind normal activation;
- make provider-specific assumptions look like core behavior.

## Safety Rules

- Import and inspect must not execute setup actions.
- Setup and materialization must be explicit, reviewable, permission-gated, and idempotent.
- Never package API keys, auth tokens, `.env` values, credential stores, provider login state, shell history, or machine-local caches.
- Secret checks may report file paths and reasons, but not matched values.
- Prefer templates, schemas, sample CSVs, and placeholder config when private source data should not travel.
- Validate every package-controlled path before reading or writing. Absolute paths and `..` traversal are unsafe unless a specific user-provided destination explicitly allows them.
- Reject or deliberately handle symlinks. Do not silently follow package symlinks into the receiver's machine.
- Keep capability declarations honest and human-readable. Do not imply enforcement exists until code actually enforces it.

## Review Questions

- What exactly can the receiver inspect before enabling or running this package?
- What files will be created, modified, or deleted?
- Can the operation be declined without partial side effects?
- Can the same operation run twice without drift?
- Are source-user data and receiver-local config clearly separated?
- Is provider-specific behavior isolated behind an adapter?

## Required Verification

Add tests for unsafe paths, secret-shaped inputs, repeated setup/materialization, and declined permission flows when those surfaces change.
