// Package snapshot is a content-addressed, immutable object store for
// package snapshots: individual file objects are stored and addressed by
// the sha256 hash of their own content, and a snapshot manifest — itself
// just another content-addressed object, in a separate namespace — records
// which object each file in a package resolves to. Because an object's
// identity is entirely a function of its bytes, identical content always
// maps to the same object ID, storage is naturally deduplicated, and
// tampering is detectable: reading an object re-verifies its content
// against the ID used to look it up (see docs/decisions/0014).
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

// ObjectID identifies an immutable object (a file's content, or a
// manifest's own serialized bytes) by content hash, formatted the same way
// packages.ComputeDigest formats package digests: "sha256:<hex>".
type ObjectID string

// CurrentManifestSchema is the current snapshot manifest format version.
// Following ADR 0005's compatibility convention, an absent schema field is
// treated as version 1; an explicitly declared zero remains unsupported.
const CurrentManifestSchema = 1

var (
	objectIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	identityPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*$`)
)

// Manifest is a deterministic, hashable description of one package
// snapshot: the package's identity, plus every file it contains mapped to
// the ObjectID holding that file's content. Because Manifest is a struct
// (not a map) and Files is kept sorted by Path, marshaling it always
// produces identical bytes for identical content — the manifest's own
// ObjectID (see Create) is the hash of exactly those bytes.
type Manifest struct {
	Schema  int            `json:"schema"`
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Files   []ManifestFile `json:"files"`
}

// ManifestFile maps one path within a package (relative to the package
// root, forward-slashed) to the object holding its content.
type ManifestFile struct {
	Path   string   `json:"path"`
	Object ObjectID `json:"object"`
}

func hashID(data []byte) ObjectID {
	sum := sha256.Sum256(data)
	return ObjectID("sha256:" + hex.EncodeToString(sum[:]))
}

// blobPath returns where an object with the given ID lives under root,
// fanned out into a two-character subdirectory (git-style) so a large
// number of objects doesn't produce one enormous flat directory.
func blobPath(root string, id ObjectID) (string, error) {
	if !objectIDPattern.MatchString(string(id)) {
		return "", fmt.Errorf("invalid object id %q", id)
	}
	hexPart := strings.TrimPrefix(string(id), "sha256:")
	return filepath.Join(root, hexPart[:2], hexPart[2:]), nil
}

// putBlob writes data as a content-addressed object under root and returns
// its ID. Writing the same content twice is a no-op the second time
// (dedup): the destination path is entirely a function of the content
// itself, so an existing file at that path is already exactly this data.
func putBlob(root string, data []byte) (ObjectID, error) {
	id := hashID(data)
	path, err := blobPath(root, id)
	if err != nil {
		return "", err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("object path %s is not a regular file", path)
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if got := hashID(existing); got != id {
			return "", fmt.Errorf("object %s is corrupt: content hashes to %s", id, got)
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return id, nil
}

// getBlob reads the object with the given ID from under root and verifies
// its content still hashes to that ID before returning it — a corrupt or
// tampered object is rejected here rather than handed to a caller that
// might reconstruct a package from it (see docs/decisions/0014).
func getBlob(root string, id ObjectID) ([]byte, error) {
	path, err := blobPath(root, id)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("object path %s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if got := hashID(data); got != id {
		return nil, fmt.Errorf("object %s is corrupt: content hashes to %s", id, got)
	}
	return data, nil
}

// WriteObject stores data as a content-addressed file object and returns
// its ID. Identical content, written any number of times, always returns
// the same ID.
func WriteObject(home string, data []byte) (ObjectID, error) {
	return putBlob(config.ObjectsDir(home), data)
}

// ReadObject returns the content of the object with the given ID,
// verifying it against the ID before returning it.
func ReadObject(home string, id ObjectID) ([]byte, error) {
	return getBlob(config.ObjectsDir(home), id)
}

// VerifyObject reports whether the object with the given ID is present and
// its stored content still hashes to that ID.
func VerifyObject(home string, id ObjectID) error {
	_, err := ReadObject(home, id)
	return err
}

// manifestBytes returns m's canonical, deterministic serialization: the
// exact bytes Create hashes to produce m's own ObjectID, and the exact
// bytes LoadManifest expects to read back.
func manifestBytes(m Manifest) ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// Create builds an immutable snapshot of the package directory at dir:
// every file under its standard content directories, plus its manifest
// file (packages.ManifestFileName), is written as a content-addressed
// object, and a Manifest listing them (sorted by path, so the result is
// deterministic across repeated calls against unchanged content) is
// written as its own object in a separate namespace. Create returns the
// Manifest and its ObjectID.
func Create(home, dir string) (Manifest, ObjectID, error) {
	manifest, err := packages.LoadManifest(dir)
	if err != nil {
		return Manifest{}, "", err
	}

	relPaths, err := packages.ContentFiles(dir)
	if err != nil {
		return Manifest{}, "", err
	}
	relPaths = append(relPaths, packages.ManifestFileName)
	sort.Strings(relPaths)

	files := make([]ManifestFile, 0, len(relPaths))
	for _, rel := range relPaths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return Manifest{}, "", fmt.Errorf("read %s for snapshot: %w", rel, err)
		}
		id, err := WriteObject(home, data)
		if err != nil {
			return Manifest{}, "", fmt.Errorf("store object for %s: %w", rel, err)
		}
		files = append(files, ManifestFile{Path: rel, Object: id})
	}

	snapshotManifest := Manifest{
		Schema:  CurrentManifestSchema,
		Name:    manifest.Name,
		Version: manifest.Version,
		Files:   files,
	}

	data, err := manifestBytes(snapshotManifest)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("encode snapshot manifest: %w", err)
	}
	id, err := putBlob(config.SnapshotsDir(home), data)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("store snapshot manifest: %w", err)
	}
	return snapshotManifest, id, nil
}

// AllManifestIDs returns the ObjectID of every snapshot manifest stored
// under home, sorted for stable output. Used by `lineage doctor` to check
// referential integrity across every snapshot this build has ever created
// (see docs/decisions/0015) without needing a separate index of what's been
// written - the fanned-out directory layout itself is the enumeration.
func AllManifestIDs(home string) ([]ObjectID, error) {
	return allObjectIDs(config.SnapshotsDir(home))
}

// allObjectIDs walks a content-addressed store's two-level fan-out
// directory layout (see blobPath) and reconstructs the ObjectID implied by
// each file's location, rather than trusting any separately maintained
// list.
func allObjectIDs(root string) ([]ObjectID, error) {
	prefixes, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []ObjectID
	for _, prefixEntry := range prefixes {
		prefix := prefixEntry.Name()
		if prefixEntry.Type()&os.ModeSymlink != 0 || !prefixEntry.IsDir() || len(prefix) != 2 || !isLowerHex(prefix) {
			return nil, fmt.Errorf("invalid content-addressed store entry %s", filepath.Join(root, prefix))
		}
		entries, err := os.ReadDir(filepath.Join(root, prefix))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 || !e.Type().IsRegular() || len(e.Name()) != 62 || !isLowerHex(e.Name()) {
				return nil, fmt.Errorf("invalid content-addressed store entry %s", filepath.Join(root, prefix, e.Name()))
			}
			ids = append(ids, ObjectID("sha256:"+prefix+e.Name()))
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func isLowerHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// LoadManifest reads and verifies the snapshot manifest with the given ID,
// then decodes it.
func LoadManifest(home string, id ObjectID) (Manifest, error) {
	data, err := getBlob(config.SnapshotsDir(home), id)
	if err != nil {
		return Manifest{}, err
	}
	var raw *Manifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("parse snapshot manifest %s: %w", id, err)
	}
	if raw == nil {
		return Manifest{}, fmt.Errorf("parse snapshot manifest %s: manifest must be a JSON object", id)
	}
	m := *raw
	var probe struct {
		Schema *int `json:"schema"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Manifest{}, fmt.Errorf("parse snapshot manifest %s: %w", id, err)
	}
	if probe.Schema == nil {
		m.Schema = CurrentManifestSchema
	}
	if err := validateManifest(m); err != nil {
		return Manifest{}, fmt.Errorf("invalid snapshot manifest %s: %w", id, err)
	}
	return m, nil
}

func validateManifest(m Manifest) error {
	if m.Schema != CurrentManifestSchema {
		return fmt.Errorf("declares schema %d, but this build only understands schema %d", m.Schema, CurrentManifestSchema)
	}
	if !identityPattern.MatchString(m.Name) {
		return fmt.Errorf("invalid package name %q", m.Name)
	}
	if !identityPattern.MatchString(m.Version) {
		return fmt.Errorf("invalid package version %q", m.Version)
	}
	if len(m.Files) == 0 {
		return fmt.Errorf("contains no files")
	}

	seen := make(map[string]struct{}, len(m.Files))
	manifestFileFound := false
	previous := ""
	for i, f := range m.Files {
		if f.Path == "" || strings.Contains(f.Path, `\`) || path.IsAbs(f.Path) || path.Clean(f.Path) != f.Path || f.Path == "." || strings.HasPrefix(f.Path, "../") {
			return fmt.Errorf("file %d has unsafe or non-canonical path %q", i, f.Path)
		}
		if _, exists := seen[f.Path]; exists {
			return fmt.Errorf("contains duplicate file path %q", f.Path)
		}
		seen[f.Path] = struct{}{}
		if previous != "" && f.Path <= previous {
			return fmt.Errorf("file paths are not in strictly sorted order")
		}
		previous = f.Path
		if f.Path == packages.ManifestFileName {
			manifestFileFound = true
		}
		if !objectIDPattern.MatchString(string(f.Object)) {
			return fmt.Errorf("file %q has invalid object id %q", f.Path, f.Object)
		}
	}
	if !manifestFileFound {
		return fmt.Errorf("does not reference %s", packages.ManifestFileName)
	}
	return nil
}

// Materialize reconstructs m's files under destDir. Every object m
// references is verified before anything is written to destDir: a single
// corrupt object fails the whole call, and destDir is left untouched,
// rather than reconstructing a package with silently missing or wrong
// content (see docs/decisions/0014).
func Materialize(home string, m Manifest, destDir string) error {
	if err := validateManifest(m); err != nil {
		return fmt.Errorf("invalid snapshot manifest: %w", err)
	}
	contents := make(map[string][]byte, len(m.Files))
	for _, f := range m.Files {
		data, err := ReadObject(home, f.Object)
		if err != nil {
			return fmt.Errorf("verify object for %s: %w", f.Path, err)
		}
		contents[f.Path] = data
	}

	for _, f := range m.Files {
		dest, err := packages.SafeJoin(destDir, f.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, contents[f.Path], 0o644); err != nil {
			return err
		}
	}
	return nil
}
