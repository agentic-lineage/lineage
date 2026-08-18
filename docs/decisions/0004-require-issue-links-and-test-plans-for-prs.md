# 0004 Require Issue Links And Test Plans For PRs

Status: Accepted

Date: 2026-08-19

## Context

As contributors join, PRs need enough context to review safely. Lineage changes can affect local files, package safety, provider launch behavior, and user trust.

## Decision

Require PRs into `develop` to link an issue and include relevant test cases or a clear explanation for why automated tests do not apply.

## Consequences

This makes review easier and keeps implementation tied to agreed work. It also creates a small amount of ceremony, but that ceremony is intentional: it protects the package distribution layer from unreviewable changes.

## Follow-Up

Keep the `PR Policy` check small. If it becomes frustrating or noisy, improve the check rather than removing the requirement.
