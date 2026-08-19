# 0012 V1 Distribution Contract And Receiver Activation

Status: Proposed

Date: 2026-08-20

## Context

The `Goal: Enable distribution` milestone (#67-#77) is the current blocker: it's what turns Lineage from "clone the repo and run it yourself" into something a real author and a real non-coder receiver can use end to end, which is a precondition for any V1 launch. Packaging (`package init` / `validate` / `export`) already works and is out of scope here.

Three open `needs:decision` questions were blocking everything downstream of them: where published packages actually live (#67, #69), how the `lineage` CLI itself gets installed (#64), and what the primary receiver activation experience is, especially for people who will never type a CLI command (#77).

An earlier version of this decision proposed a custom backend with its own blob storage and metadata database. That was revised before any of it was built: GitHub Releases already provide versioned, asset-hosting storage for free, and a private repo gives access control without building an auth/permissions system from scratch. The revision below reflects that.

## Decision

1. **Hosting/storage.** Package artifacts live in one shared **private GitHub repo**, [`agentic-lineage/published-artifacts`](https://github.com/agentic-lineage/published-artifacts). Publishing a package creates a GitHub Release in that repo with the validated `.tgz` and manifest attached as release assets. No custom blob storage or metadata database — GitHub *is* the storage layer.

2. **Website's role.** The existing `landing/` Vercel deployment is extended with a thin API layer, not a storage layer. It holds the one GitHub token with write access to the private registry repo. Publishers authenticate to the website's publish API with their own bearer token; the website is the only thing that ever talks to GitHub. Receivers (CLI pull, `lineage add`, and the bootstrap prompt) call the website's read API, which proxies the asset out of the private repo — no publisher or receiver ever needs their own GitHub credentials. `landing/` becomes the "Lineage website" referenced by #69, rather than a new `website/` directory or a separate codebase.

3. **Versioning.** Version identity is **not** tied directly to GitHub. It's the package's own manifest-declared version (`lineage.yaml`) plus the content digest, matching 0005 (manifest is authoritative, digest is the integrity check). A GitHub Release tag of the form `<package-name>@<version>` is used as where V1 happens to keep the bytes and gives a free, human-browsable version history, but the CLI always verifies against the manifest version and digest, never trusts the tag string as version of record. This keeps the package/version model portable if storage ever moves off GitHub — only the storage layer would change.

4. **CLI install mechanism.** `npx @lineage/cli <command>` is the one-command install/run path for V1. No GoReleaser build matrix or hosted `curl | sh` install script for V1 — that's deferred, matching #64's own non-goals section. A non-coder receiver never needs to know this command exists; the bootstrap prompt (below) runs it on their behalf.

5. **Receiver activation, primary path.** The primary documented flow (README, website) is a short, copy-pasteable instruction block that a non-coder pastes into their coding agent's chat (Claude, Codex, etc.). The agent uses its own shell access to: run the npx-based install, fetch the referenced package, inspect/validate it, walk through permission-gated setup, and enable it — leaving the user with an active, ready-to-run workflow and the exact command to spawn/run it next. `lineage add <package-ref>` (#77) is the direct CLI command underlying this flow and stays documented as the path for people who already have Lineage installed.

6. **Publisher identity and package ownership.** Superseded by decision 6b below — kept here for history. The first version of this decision used a flat, admin-minted `LINEAGE_PUBLISH_TOKENS` mapping (`token:publisher-id` pairs), which meant every new publisher required a manual env var edit and a redeploy. That doesn't grow.

6b. **Publisher identity is a verified GitHub login, not an admin-minted token.** Since the whole registry already lives on GitHub, publisher identity does too: `lineage login` runs GitHub's OAuth Device Flow (the same mechanism `gh auth login` uses — no client secret required, safe for a public/native CLI), scoped to `read:user` only, and stores the resulting GitHub access token locally. `lineage package publish` sends that token as the request's bearer token; the website verifies it by calling `GET https://api.github.com/user` and uses the verified `login` it gets back as the publisher id — never a client-supplied string. Ownership works exactly as before (first publish of a name claims it for that login; a later publish from a different verified login is rejected, 403), but now adding a publisher requires zero action from the site operator: anyone with a GitHub account can `lineage login` and publish. Publish is **open** — any verified GitHub account can publish a new package name — matching how npm/crates.io/PyPI work, rather than gating behind a pre-approved list. `LINEAGE_PUBLISH_TOKEN` remains as an escape hatch for non-interactive use (CI): set it to any GitHub-issued token with `read:user` access and the website verifies it exactly the same way, no separate CI-specific mechanism needed.

7. **Publish is interruption-safe.** Publish is commonly agent-driven — an agent shells out to `lineage package publish` on the author's behalf — and that agent's own session/usage can end mid-command, same as a network drop or a serverless function timeout. The website creates the GitHub Release as a **draft**, uploads the archive asset to it, and only then finalizes it (`draft: false`). A publish interrupted after the release exists but before its asset lands leaves a draft, which is invisible everywhere a receiver could see it (list, resolve-by-ref, download all filter out drafts) and isn't yet "owned" by anyone. Retrying the identical publish call finds that draft by tag and resumes it — re-uploads the asset, finalizes — instead of failing on "tag already exists" or leaving an orphaned duplicate. This makes "just run the same command again" the correct recovery for any interruption, without needing to know why it was interrupted.

## Consequences

- Publish (#68) needs an authenticated upload path against the website's publish API instead of a raw `git push`; the website does the actual GitHub Release creation server-side. Identity comes from GitHub's own OAuth device flow (decision 6b), so the website never mints or stores publisher credentials itself — it only ever verifies a token GitHub already issued.
- `lineage login`/`lineage logout` are new CLI surface, and publish's failure mode when nobody's logged in (and `LINEAGE_PUBLISH_TOKEN` isn't set) needs to say so clearly rather than a bare 401 from the website.
- Pull (#70) and unified artifact processing (#71) resolve package refs against the website's read API, which resolves `<package-name>@<version>` to a Release asset in the private repo. Digest verification stays the CLI's job regardless of transport — website/API metadata is advisory, the package digest is authoritative (already an acceptance criterion on #67).
- Rate limits and API surface are now GitHub's (Releases API, asset download), not something Lineage has to scale itself — but it also means Lineage inherits GitHub's availability and API limits as a dependency.
- The bootstrap prompt is a content artifact, not just a command: it needs to be written, versioned alongside the CLI, and reviewed for prompt-injection safety, since it's pasted into an agent that has real tool access on the receiver's machine.
- Provisioning is now small: create one private GitHub repo, mint a token with release-write access to it, set it as an env var on the website deployment. That still needs the project owner's action before publish/pull can be exercised end to end, but there's no blob store or database to stand up.
- `lineage add` (#77), `pull` (#70), and the bootstrap prompt all converge on the same validate → inspect → permission-gated setup → enable state machine already required by #71; none of them get a separate code path.
- An abandoned draft (an interrupted publish that's never retried) is a small amount of invisible clutter in the registry repo's release list — acceptable for V1, revisit with a cleanup job only if it becomes a real problem.

## Follow-Up

- Who holds the `LINEAGE_REGISTRY_TOKEN` used by the website against `agentic-lineage/published-artifacts` still needs to be settled and rotated on a schedule.
- Publish is fully open for V1 (decision 6b) — no pre-vetting of who can claim a package name. If spam/abuse becomes real, the next lever is a lightweight allowlist (e.g. a file committed to the registry repo itself, so it still doesn't need a website redeploy to update) rather than reverting to admin-minted tokens.
- ~~The GitHub OAuth App backing `lineage login` needs to be registered (org: `agentic-lineage`, Device Flow enabled) and its Client ID compiled into the CLI.~~ Done — registered, Device Flow enabled, Client ID compiled in. Still untested: a real human completing the browser approval step end to end (verified so far: device code request/response against the live endpoint, and the CLI's own prompt).
- Write and security-review the actual bootstrap prompt text once the website's endpoints exist to fetch/inspect/enable against.
- Revisit the deferred GoReleaser/`curl | sh` install path (#64) once npx-based install proves insufficient (e.g. environments without Node).
- If a single shared registry repo becomes a namespace or scale problem, revisit per-package or per-publisher repos — the version/digest model above should not need to change if that happens.
