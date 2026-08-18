# 0001 Use Go For The Local Runtime

Status: Accepted

Date: 2026-08-19

## Context

Lineage is a local runtime that should be easy to install, quick to start, and able to manage files, subprocesses, package archives, and provider shims across platforms.

## Decision

Use Go for the core CLI/runtime.

## Consequences

This gives Lineage a portable single-binary path, strong filesystem/process support, fast startup, and a straightforward test story with `go test ./...`.

It also means contributors should avoid adding runtime dependencies unless they clearly improve local reliability or safety.

## Follow-Up

Keep the source layout idiomatic: `cmd/` for entrypoints and `internal/` for runtime packages.
