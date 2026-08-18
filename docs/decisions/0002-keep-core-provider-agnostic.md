# 0002 Keep Core Provider-Agnostic

Status: Accepted

Date: 2026-08-19

## Context

Lineage packages need to distribute agent environments while different providers may expose different commands, config locations, session concepts, and native extension points.

## Decision

Keep the core package, config, runtime, validation, and materialization semantics provider-agnostic. Provider-specific behavior belongs behind explicit adapter or shim boundaries.

## Consequences

This protects Lineage from baking one provider's assumptions into the package format. It also makes provider work slightly more deliberate: new provider behavior must state which boundary it belongs behind.

## Follow-Up

Provider adapter PRs should explain which files, environment variables, or command arguments they touch and why that behavior does not belong in core package semantics.
