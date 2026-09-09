# 0017 Package Content Addressing Has A Complete, Verifiable Install Contract

Status: Accepted

Date: 2026-09-09

## Context

ADR 0014 already provides a local content-addressed snapshot store. It is a
useful implementation precedent, but it is not the distribution contract:
snapshots are created while enabling a package, do not describe package-install
completeness, and cannot tell a registry client which immutable bodies are
missing. Without a single contract, local caching, registry transfer, and
inspection could each assign different meanings to a digest or to an installed
package.

Issue #264 needs packages to transfer immutable bodies once per digest while
retaining `name@version` as the release identity. This decision defines that
boundary for #266, #267, and #268.

## Decision

### Identities and scope

- A package release is identified for users and registries by `name@version`.
  A content digest never replaces that identity.
- An immutable content object is the exact byte sequence of one regular source
  package file. Its only supported identifier is `sha256:<64 lowercase hex>`;
  the digest is SHA-256 over those bytes, with no path, mode, package name, or
  version prefix.
- The CAS covers `lineage.yaml` and regular files below the standard
  distributable directories: `skills`, `workflows`, `agents`, `policies`,
  `references`, and `adapters`. Symlinks are rejected, as they are for current
  discovery and archive import/export. Empty directories are not objects.
- Executable mode, owner, timestamps, and symlink targets are deliberately not
  package-content semantics in this schema. Materialization continues to write
  regular non-executable files. Generated provider artifacts, local setup
  output, `.lineage` state, credentials, and registry metadata are excluded.

### Versioned package content manifest

The registry-facing content manifest is a deterministic, versioned document.
It is serialized with a sorted `assets` list; maps must not be used for
canonical fields. Schema 1 has this shape:

```yaml
schema: 1
name: example-package
version: 1.2.0
package_digest: sha256:<whole-package-digest>
assets:
  - path: lineage.yaml
    kind: manifest
    digest: sha256:<object-digest>
    bytes: 284
    media_type: application/yaml
  - path: skills/review/SKILL.md
    kind: skill
    digest: sha256:<object-digest>
    bytes: 1943
    media_type: text/markdown
```

- `path` is a unique, relative, forward-slashed path and must pass the same
  safe-join validation used for package-controlled materialization.
- `kind` is derived from the top-level package directory (or `manifest` for
  `lineage.yaml`); it aids inspection but does not change object identity.
- `digest` and `bytes` are required and are checked against downloaded or
  stored bytes. `media_type` is optional, advisory, and must never determine
  how a file is executed.
- `package_digest` is the existing deterministic digest defined by ADR 0005.
  It verifies the reconstructed package as a whole; asset digests verify each
  body independently. A consumer must validate both when each is available.

Manifest bytes themselves may later be content-addressed, but their transport
identity is intentionally separate from the asset schema. This avoids making a
registry's manifest envelope part of a package's source-content identity.

### Store, lifecycle, and completeness

- The local object namespace is `LINEAGE_HOME/objects/sha256/aa/<remaining-hex>`
  conceptually. Its fan-out is an implementation detail; the object ID format,
  immutability, and verification are contractual. ADR 0014's existing object
  store is the migration target for this namespace.
- A writer hashes incoming bytes before choosing the destination, writes to a
  same-directory temporary file, fsyncs/closes it as supported, then atomically
  renames it into place. If the destination already exists, it is re-verified;
  a corrupt existing object is an error, never a cache hit. Concurrent writers
  may race but must converge on one verified object and never expose a partial
  object.
- An installed release has a reference record separate from objects. It is
  **complete** only after its content manifest validates and every referenced
  object is present and digest-verified. The reference record is atomically
  committed only then. Normal `install`/`add` therefore remains fully usable
  offline after success.
- A failed download may retain objects that independently verified, but it
  must not replace a prior complete release or advertise the target as
  installed. A cache miss, digest mismatch, malformed manifest, or corrupt
  object is an explicit failure. Materialization resolves only verified objects
  from a complete release and fails closed before writing package content.
- Installed-release reference records are the liveness roots for future garbage
  collection. Snapshot manifests and any explicit pins are additional roots.
  No collector may delete a referenced object; eviction policy is deferred.

### Compatibility and migration

Current `.tgz` import/pull remains a supported full-package transport during
migration. It is validated using the existing archive and whole-package digest
checks, then a content manifest can be deterministically synthesized from the
verified extracted package and its files admitted to the object store. Existing
installed directories remain readable until that migration completes; callers
must not require a CAS reference merely to use a previously valid install.

For a new content-manifest-capable registry, a client validates the manifest,
compares its asset digest set with verified local objects, fetches only missing
objects, and commits the installed-release reference only once all assets are
present. Authorization controls whether a body can be fetched; a digest never
authorizes access or proves package ownership.

### Boundaries for dependent work

- #266 implements the common object-store and installed-release reference APIs,
  including atomic writes, verification, migration, and safe lookup.
- #267 uses those APIs to transfer missing digests. It does not introduce a
  binary-diff protocol or let a partial install masquerade as complete.
- #268 reports exact manifest/object byte data and derived context estimates.
  Token estimates have a named estimator/version and are not manifest identity
  fields or universal runtime facts.
- Provider adapters receive resolved verified files; they do not implement CAS
  storage, fetch, or completeness rules.

## Consequences

Identical source files can be shared across package names and versions without
making package/version names ambiguous. The contract also forces a sharp
distinction between verified cached objects and an offline-usable installed
release, which protects a working installation when an update fails.

The current snapshot implementation must be hardened before it becomes the
shared store: specifically strict object-ID validation and atomic writes. That
work belongs to #266, not this design issue. Registry and CLI clients must
support the legacy archive path while the manifest endpoint is introduced.

## Follow-Up

- Implement #266 against this contract, including corruption, concurrent-write,
  safe-path, migration, and cross-package-dedup tests.
- Implement #267's missing-digest registry flow after #266 exposes a verified
  object-presence API.
- Define #268's diagnostic schema separately; it may use asset `bytes` but
  must identify any token estimator and distinguish estimates from facts.
- Revisit object algorithm agility only with a versioned manifest schema and a
  migration plan; do not accept ambiguous bare digests.
