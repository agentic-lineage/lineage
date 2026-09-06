# Testing

Run the full test suite:

```bash
go test ./...
```

Before merging package-format or setup-flow changes, add tests for:

- Deterministic manifest output.
- Idempotent command behavior.
- Package paths that try to escape the package or workspace root.
- Package contents that look like secrets.
- Read-only source-workspace inventory, including deterministic classification,
  digests, citations, and ambiguous references.
- Receiver-side setup prompts that must not run without explicit permission.
- Registry receiver paths such as `lineage add` and `lineage package pull`,
  including digest mismatch and repeated-run behavior.
- Public docs or bootstrap prompt changes that must stay synchronized with the
  website or package pages. If automated drift checks do not exist yet, note the
  manual sync target in the PR.

The checked-in author-to-receiver flow is an end-to-end fixture; keep it useful
for confirming the package loop across platforms, including Windows CI.
