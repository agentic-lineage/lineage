# Public Docs Sync Checklist

Lineage has several public documentation surfaces. The repository should stay
the canonical source, but users often arrive through the website, Wiki, package
pages, or Discussions first. When a change affects install, package publishing,
receiver activation, setup prompts, provider compatibility, or safety wording,
check these surfaces before calling the docs fresh.

## Canonical Repo Sources

- `README.md`: current install, author, receiver, registry, and day-to-day CLI
  flow.
- `ROADMAP.md`: shipped work, active priorities, and explicitly unshipped
  provider work.
- `CHANGELOG.md`: release history and complete behavior notes.
- `docs/architecture.md`: system boundaries and trust model.
- `docs/safety.md`: canonical safety model - what each command checks, what's
  a warning vs. a block, and what's explicitly not implemented yet.
- `docs/bootstrap-prompt.md`: canonical copy-paste prompt embedded by package
  pages.
- `docs/guides/`: search-targeted explanations for package concepts and
  released provider workflow sharing, plus contributor-facing compilation
  design where relevant.
- `llms.txt`: concise agent-readable repository summary and canonical links.
- `docs/discoverability.md`: GitHub SEO, website SEO, GEO/AEO, metadata,
  sitemap, social preview, and search-targeted content checklist.
- `docs/decisions/`: rationale for accepted design decisions.

## Public Surfaces To Sync

- Website homepage: should summarize the current product surface, including the
  Supabase-backed registry, package pages, `lineage add`, and provider
  run/preview paths.
- Website SEO files: `robots.txt`, `sitemap.xml`, `sitemap.md`, `llms.txt`,
  canonicals, structured data, and Open Graph/Twitter metadata should stay in
  sync with current routes and package registry data.
- `/packages`: should describe the registry read path, use the paginated
  `/api/packages` response, and display package provider compatibility,
  declared capabilities, publisher, digest-adjacent trust signals, and
  aggregate usage metrics when available.
- `/packages/<name>`: should show the latest resolved package version, digest,
  publisher, direct pull command, archive download, bootstrap prompt,
  page-specific canonical/OG metadata, and package-specific structured data.
- GitHub Wiki: should mirror the README-level usage flow, especially install,
  publish/pull, `lineage add`, inspect-before-enable, workflow run, and safety
  rules.
- GitHub Discussions: seed threads should point new users to the README,
  package directory, Wiki usage page, and bootstrap prompt rather than
  maintaining a separate stale command list.

## Known Drift To Watch

- Package detail pages must agree with `/api/packages` on version and digest.
- `sitemap.xml` must include `/packages` and every published package detail
  page, not just the homepage.
- `/packages` and package detail pages should use page-specific `og:url` values
  that match their canonical URLs.
- The website embeds the bootstrap prompt by hand; compare it with
  `docs/bootstrap-prompt.md` whenever either changes.
- Provider compatibility and capabilities currently depend on registry fields
  being published and rendered end to end. If only the CLI side has landed,
  public copy should say that website rendering is pending.
- Do not add an adapter to public provider lists, bootstrap prompts, or package
  pages while its PR is open. Merge the adapter first, then synchronize every
  affected public surface in the same release-tracked change.
- Registry storage copy should describe Supabase as the active production store
  and GitHub as publisher identity plus rollback/backfill context. If the
  website storage adapter changes again, update README, architecture, ADR 0012,
  public docs, package pages, and the Wiki together.
- Release/versioning policy lives in the repo and should be linked from
  contributor docs once the release-tracking PR lands.
