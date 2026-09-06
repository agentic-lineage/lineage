# Discoverability Checklist

Lineage has two discoverability surfaces: the GitHub repository and the public
website at `agenticlineage.vercel.app`. Keep both specific to Lineage's actual
product shape: local packages for agent skills, workflows, agents, policies,
references, setup material, and provider adapters.

The goal is not keyword stuffing. The goal is to make the project easy for
people, search engines, and answer engines to classify correctly.

## Search Positioning

Use this category language consistently in high-signal places:

- Lineage is a local distribution layer for AI agent workflows.
- Lineage packages and shares Claude Code workflows, Codex workflows, skills,
  agents, policies, references, and provider adapters.
- A Lineage package is inspectable before it is enabled and runs through the
  receiver's own local agent tools.

Use these phrases naturally where they describe real functionality:

- AI agent workflows
- Claude Code workflows
- Codex workflows
- agent skills
- reusable agent workflows
- agent workflow packages
- local agent runtime
- `lineage.yaml`

Avoid vague claims such as "the future of agents", "all-in-one AI platform", or
"enterprise-ready automation" unless the product surface actually supports them.

## GitHub Repository

Recommended repository description:

```text
Package and share AI agent workflows, skills, agents, and policies across Claude Code, Codex, and other coding agents.
```

Recommended GitHub topics, staying under GitHub's 20-topic limit:

```text
ai-agents
agentic-ai
agent-workflows
ai-workflows
agent-skills
claude-code
codex
developer-tools
cli
golang
workflow-automation
open-source
```

Repository README checks:

- The first paragraph says what category Lineage belongs to.
- The first screen mentions Claude Code, Codex, workflows, skills, and packages.
- Install and quick-start commands are copyable and current.
- Safety wording makes package inspection and permission-gated materialization
  visible before usage examples get too deep.
- Important docs are linked with descriptive anchor text, not just "here".
- The repository links back to the website package directory.

Social preview:

- Use a 1280x640 or 1200x630 image with the Lineage name and the phrase
  "Package AI agent workflows".
- Avoid screenshots that depend on tiny terminal text.
- Keep the GitHub social preview distinct from the scenic website hero so a
  shared repository link communicates the developer-tool category immediately.

## Website Technical SEO

Every public HTML route should have:

- Exactly one canonical URL using `https://agenticlineage.vercel.app/...`.
- A unique title and meta description written for the page's actual job.
- Matching Open Graph and Twitter metadata, with `og:url` equal to the page
  canonical.
- JSON-LD that matches the visible page content.
- `meta name="robots" content="index, follow"` unless the page is intentionally
  private or thin.
- Internal links back to the repository, Wiki, Discussions, package directory,
  architecture, and bootstrap prompt where relevant.

Current live-site audit notes from 2026-08-24:

- `/robots.txt` returns 200, allows all crawlers, and points to
  `/sitemap.xml`.
- `/llms.txt` returns 200 as plain text and summarizes the project for answer
  engines.
- `/` has a canonical URL, meta description, Open Graph/Twitter metadata, and
  JSON-LD for `WebSite`, `SoftwareSourceCode`, and `FAQPage`.
- `/packages` has a canonical URL and indexable metadata, but its `og:url`
  should be page-specific instead of the homepage.
- `/packages/<name>` has package-specific title, description, canonical URL,
  and social metadata. Add package-specific JSON-LD when the website source is
  updated.
- `/sitemap.xml` currently lists only the homepage. It should also include
  `/packages` and every published package detail page.

## Website Sitemap And Agent Files

The XML sitemap should include all indexable public pages:

```xml
<url>
  <loc>https://agenticlineage.vercel.app/</loc>
  <lastmod>2026-08-23T21:00:00.918Z</lastmod>
</url>
<url>
  <loc>https://agenticlineage.vercel.app/packages</loc>
  <lastmod>2026-08-23T21:00:00.918Z</lastmod>
</url>
<url>
  <loc>https://agenticlineage.vercel.app/packages/ai-product-engineer-resume</loc>
  <lastmod>2026-08-22T00:00:00.000Z</lastmod>
</url>
```

If package pages are generated from registry data, generate sitemap entries from
that same registry source. Use each package's latest publish/update time as
`lastmod`.

Add a markdown sitemap at `/sitemap.md` for agent readability:

```markdown
# Lineage Sitemap

## Product

- [Home](https://agenticlineage.vercel.app/): What Lineage is and how it fits Claude and Codex workflows.
- [Packages](https://agenticlineage.vercel.app/packages): Published Lineage packages.

## Repository Documentation

- [README](https://github.com/agentic-lineage/lineage): Install, quick start, package shape, and safety principles.
- [Architecture](https://github.com/agentic-lineage/lineage/blob/develop/docs/architecture.md): Runtime boundaries and trust model.
- [Bootstrap prompt](https://github.com/agentic-lineage/lineage/blob/develop/docs/bootstrap-prompt.md): Canonical receiver prompt for package pages.
```

Keep `/llms.txt` concise and link to canonical docs. If the website adds
markdown mirrors for HTML pages, link them from `/llms.txt` and add
`rel="alternate" type="text/markdown"` from the HTML routes.

## Search-Targeted Content

Create content only when it answers a real query better than the homepage can.
Current repository pages:

- [What is a Lineage package?](guides/lineage-package.md)
- [How to share Claude Code and Codex workflows](guides/share-agent-workflows.md)
- [Compiling an existing agent workspace](guides/compiling-existing-workspaces.md)

Future candidates:

- `Lineage package manifest: lineage.yaml`
- `Why package an agent workflow instead of sharing a prompt?`
- `How Lineage keeps package activation inspectable`

Each page should start with the direct answer in one or two sentences, then show
commands, package structure, safety constraints, and links to the relevant repo
docs. Keep the writing factual and product-specific.

## Verification Commands

Use these checks after every website release:

```bash
curl -sS -i https://agenticlineage.vercel.app/robots.txt
curl -sS -i https://agenticlineage.vercel.app/sitemap.xml
curl -sS -i https://agenticlineage.vercel.app/llms.txt
curl -sS -i https://agenticlineage.vercel.app/
curl -sS -i https://agenticlineage.vercel.app/packages
```

Confirm that live pages return 200, do not include `noindex`, expose canonical
URLs, and are present in either `sitemap.xml`, `sitemap.md`, `llms.txt`, or a
linked discoverable page.

## Current Gaps

These items are not fully fixable in this repository because the website source
is maintained separately:

- Generate `sitemap.xml` entries for `/packages` and each published package
  detail page.
- Add `/sitemap.md` to the website and link it from `/llms.txt`.
- Make `/packages` use a page-specific `og:url` matching its canonical URL.
- Add package-specific JSON-LD to `/packages/<name>` pages.
- Add `rel="alternate" type="text/markdown"` if the website ships markdown
  mirrors for HTML pages.
