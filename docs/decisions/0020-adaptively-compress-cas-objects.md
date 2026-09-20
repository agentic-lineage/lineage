# 0020 Adaptively Compress CAS Objects Without Changing Their Identity

Status: Accepted

Date: 2026-09-20

## Context

ADR 0017 gives each source file an identity computed from its exact bytes and
deduplicates equal files in the local content-addressed store. The first store
implementation writes those bytes verbatim. That keeps the contract simple,
but it leaves repeated Markdown, YAML, JSON, and CSV syntax uncompressed and
therefore does not make typical workflow packages as small on disk as they can
be.

The relevant research points in a consistent direction:

- Zstandard combines dictionary/LZ matching with entropy coding, supports
  independent frames and fast decoding, and standardizes checksummed frames in
  RFC 8878.
- Shared-reference research shows why a pre-existing dictionary helps small
  files: a short input cannot build much useful history by itself. Zstandard's
  trained dictionaries operationalize that result for groups of similar small
  records.
- FastCDC improves deduplication for large files whose boundaries shift, but
  Lineage already addresses each usually-small source file independently.
  Splitting those files again would add chunk IDs, indexes, and extra reads
  before there is evidence that shifted large-file content dominates package
  weight.
- Git-style delta packs and lazy container formats are strong transport or
  cold-start techniques. Making either the canonical local representation
  would introduce dependency chains or network availability into random object
  reads, weakening ADR 0017's offline completeness contract.

References:

- RFC 8878, Zstandard Compression and the `application/zstd` Media Type:
  <https://www.rfc-editor.org/rfc/rfc8878>
- Xia et al., FastCDC: a Fast and Efficient Content-Defined Chunking Approach
  for Data Deduplication:
  <https://www.usenix.org/system/files/conference/atc16/atc16-paper-xia.pdf>
- Klein and Shapira, Compression in the Presence of Shared Data:
  <https://doi.org/10.1016/S0020-0255(01)00099-8>
- Git pack-object format and delta behavior:
  <https://git-scm.com/docs/git-pack-objects>
- eStargz lazy pulling and prioritized prefetch:
  <https://github.com/containerd/stargz-snapshotter/blob/main/docs/estargz.md>

## Local Experiment

The five source assets in `examples/resume-workflow` total 1,346 bytes. A
whole-package tar introduces substantial fixed metadata before compression.
Compressing each logical object independently produced these totals, excluding
the unchanged digest index:

| Representation | Bytes |
| --- | ---: |
| Raw objects | 1,346 |
| Per-object gzip | 1,038 |
| Per-object Zstandard, default level | 954 |
| Per-object Brotli, quality 6 | 809 |
| Per-object Zstandard with a trained 4 KiB dictionary | 770 |

Brotli produced the smallest standalone bodies in this very small sample.
Zstandard is selected because decoding is designed for high throughput, its
frames support checksums and bounded decoding, and it leaves a compatible path
to shared dictionaries. The dictionary result is not adopted yet: 770 bytes
plus a package-specific 4,096-byte dictionary is a net loss. A dictionary only
wins after a stable, versioned dictionary is shared across enough packages.

## Decision

- Object IDs remain SHA-256 over the original uncompressed bytes. Content
  manifests, registry verification, deduplication, and reconstructed package
  output do not change.
- Legacy and incompressible objects remain at the existing raw object path.
  Compressed objects use a separate `zstd-v1` representation namespace. The
  namespace, not a byte prefix, identifies the encoding, so arbitrary legacy
  content cannot be mistaken for an envelope.
- A new object is compressed independently with Zstandard's default level only
  when it is between 128 bytes and 50 MiB and the result saves at least 16 bytes
  and 10 percent. This avoids expanding tiny, binary, encrypted, or already
  compressed content.
- Reads prefer an existing raw object for backward compatibility, otherwise
  decode `zstd-v1`, then verify the original SHA-256. A corrupt raw object is an
  error even if a second representation exists; redundant storage must not hide
  tampering.
- The decoder limits output and window memory to 50 MiB. Larger local objects
  remain supported in raw form, matching the existing 50 MiB registry-object
  boundary without creating a decompression-bomb path.
- Weight inspection reports logical verified bytes, physical CAS bytes, saved
  bytes, and each verified asset's local encoding. Existing logical package and
  context-weight fields retain their meanings.
- Snapshot manifest blobs remain raw. They live in a separate namespace, are
  small, and are not package content objects.

## Consequences

Existing stores remain readable with no migration. New clients may create
compressed objects that older clients cannot discover, so the new
representation is a local forward-compatibility feature rather than a registry
wire format. If an old and new client both write the same digest, both physical
representations can exist; reads remain deterministic and correct.

Compression adds a Go dependency, about 916 KiB of vendored source, and about
562 KiB (5.4 percent) to the current stripped macOS arm64 CLI. The runtime
encoder and decoder are reused, and buffer operations are safe for concurrent
callers. Compression happens only on object admission; reads pay one bounded
decode before the same digest verification already required by the store. This
fixed client cost is justified only when CAS objects are reused across many
packages and versions; the physical-byte diagnostics make that tradeoff
measurable.

## Follow-Up

- Measure a representative multi-package corpus before publishing a shared
  dictionary. Any dictionary must have a stable ID, versioned bytes, broad
  amortization, and a raw fallback.
- Carry Zstandard at the HTTP content-encoding or object-bundle layer so
  registry transfer benefits without changing raw object identity.
- Continue with CAS-native package resolution to remove expanded package and
  provider-copy duplication. Compression reduces object bytes but does not by
  itself eliminate those copies.
- Reconsider content-defined chunking only if measurements show that large,
  slightly edited references dominate storage across versions.
