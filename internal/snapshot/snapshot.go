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

	"github.com/agentic-lineage/lineage/internal/atomicfile"
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

// CurrentContentManifestSchema is the current installed-package content
// manifest format defined by ADR 0017.
const CurrentContentManifestSchema = 1

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

// ContentManifest describes the immutable source files required by one
// package release. It is the release reference written only after every
// listed object has been verified in the local store.
type ContentManifest struct {
	Schema        int            `json:"schema"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	PackageDigest string         `json:"package_digest"`
	Assets        []ContentAsset `json:"assets"`
}

// ContentAsset maps a package-controlled logical path to one immutable
// object, retaining the exact byte count required by package inspection.
type ContentAsset struct {
	Path      string   `json:"path"`
	Kind      string   `json:"kind"`
	Object    ObjectID `json:"digest"`
	Bytes     int64    `json:"bytes"`
	MediaType string   `json:"media_type,omitempty"`
}

func hashID(data []byte) ObjectID {
	sum := sha256.Sum256(data)
	return ObjectID("sha256:" + hex.EncodeToString(sum[:]))
}

// ObjectIDFor returns the canonical identity for data. Registry clients use
// it to reject mismatched downloads before admitting any bytes to the CAS.
func ObjectIDFor(data []byte) ObjectID {
	return hashID(data)
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
	if err := writeBlobNoClobber(path, data); err != nil {
		return "", err
	}
	if _, err := getBlob(root, id); err != nil {
		return "", err
	}
	return id, nil
}

// writeBlobNoClobber publishes a fully written temporary file with a hard
// link, whose destination-exists behavior is atomic. A concurrent winner is
// re-verified by putBlob rather than overwritten.
func writeBlobNoClobber(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".object-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpPath, path); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	return nil
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
	return putObject(config.ObjectsDir(home), data)
}

// ReadObject returns the content of the object with the given ID,
// verifying it against the ID before returning it.
func ReadObject(home string, id ObjectID) ([]byte, error) {
	data, _, err := getObject(config.ObjectsDir(home), id)
	return data, err
}

// VerifyObject reports whether the object with the given ID is present and
// its stored content still hashes to that ID.
func VerifyObject(home string, id ObjectID) error {
	_, err := ReadObject(home, id)
	return err
}

// ObjectStatus describes whether an object can be used as verified local
// content. Missing and corrupt objects are deliberately distinct so a
// caller cannot mistake corruption for a cache miss.
type ObjectStatus int

const (
	ObjectMissing ObjectStatus = iota
	ObjectVerified
	ObjectCorrupt
)

// ObjectAvailability verifies id's stored bytes when present. Invalid IDs
// are errors because they are malformed manifest input, not cache misses.
func ObjectAvailability(home string, id ObjectID) (ObjectStatus, error) {
	status, _, err := ObjectStorageInfo(home, id)
	return status, err
}

// BuildContentManifest derives ADR 0017's deterministic package content
// manifest from an already-valid package directory. It does not write data.
func BuildContentManifest(dir string) (ContentManifest, error) {
	pkg, err := packages.Discover(dir)
	if err != nil {
		return ContentManifest{}, err
	}
	relPaths, err := packages.ContentFiles(dir)
	if err != nil {
		return ContentManifest{}, err
	}
	relPaths = append(relPaths, packages.ManifestFileName)
	sort.Strings(relPaths)
	assets := make([]ContentAsset, 0, len(relPaths))
	for _, rel := range relPaths {
		data, err := readRegularPackageFile(dir, rel)
		if err != nil {
			return ContentManifest{}, fmt.Errorf("read %s for content manifest: %w", rel, err)
		}
		assets = append(assets, ContentAsset{
			Path:      rel,
			Kind:      contentKind(rel),
			Object:    hashID(data),
			Bytes:     int64(len(data)),
			MediaType: contentMediaType(rel),
		})
	}
	m := ContentManifest{
		Schema:        CurrentContentManifestSchema,
		Name:          pkg.Manifest.Name,
		Version:       pkg.Manifest.Version,
		PackageDigest: pkg.Digest,
		Assets:        assets,
	}
	if err := validateContentManifest(m); err != nil {
		return ContentManifest{}, err
	}
	return m, nil
}

// StorePackage admits every source asset from dir to the object store and
// returns its content manifest. It does not mark the release installed;
// callers must use CommitRelease after all objects are present.
func StorePackage(home, dir string) (ContentManifest, error) {
	m, err := BuildContentManifest(dir)
	if err != nil {
		return ContentManifest{}, err
	}
	for _, asset := range m.Assets {
		data, err := readRegularPackageFile(dir, asset.Path)
		if err != nil {
			return ContentManifest{}, fmt.Errorf("read %s for object store: %w", asset.Path, err)
		}
		id, err := WriteObject(home, data)
		if err != nil {
			return ContentManifest{}, fmt.Errorf("store object for %s: %w", asset.Path, err)
		}
		if id != asset.Object {
			return ContentManifest{}, fmt.Errorf("store object for %s: expected %s, got %s", asset.Path, asset.Object, id)
		}
	}
	return m, nil
}

func readRegularPackageFile(dir, rel string) ([]byte, error) {
	path, err := packages.SafeJoin(dir, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	return os.ReadFile(path)
}

// CommitRelease atomically records m as installed only when every referenced
// object is already present and verified. A failed attempt leaves any prior
// complete release reference unchanged.
func CommitRelease(home string, m ContentManifest) error {
	if err := validateContentManifest(m); err != nil {
		return fmt.Errorf("invalid package content manifest: %w", err)
	}
	if err := verifyContentManifestObjects(home, m); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installed release: %w", err)
	}
	path, err := releasePath(home, m.Name, m.Version)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("commit installed release: %w", err)
	}
	return nil
}

// LoadRelease returns a complete installed release reference and re-verifies
// every object before allowing its manifest to be used.
func LoadRelease(home, name, version string) (ContentManifest, error) {
	path, err := releasePath(home, name, version)
	if err != nil {
		return ContentManifest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ContentManifest{}, err
	}
	var m ContentManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return ContentManifest{}, fmt.Errorf("parse installed release %s@%s: %w", name, version, err)
	}
	if err := validateContentManifest(m); err != nil {
		return ContentManifest{}, fmt.Errorf("invalid installed release %s@%s: %w", name, version, err)
	}
	if m.Name != name || m.Version != version {
		return ContentManifest{}, fmt.Errorf("installed release %s@%s has mismatched identity %s@%s", name, version, m.Name, m.Version)
	}
	if err := verifyContentManifestObjects(home, m); err != nil {
		return ContentManifest{}, err
	}
	return m, nil
}

// MigrateLegacyInstall converts an existing validated package directory into
// a complete local content-addressed release without changing that directory.
func MigrateLegacyInstall(home, dir string) (ContentManifest, error) {
	m, err := StorePackage(home, dir)
	if err != nil {
		return ContentManifest{}, err
	}
	if err := CommitRelease(home, m); err != nil {
		return ContentManifest{}, err
	}
	return m, nil
}

// MaterializeRelease reconstructs a complete installed release at destDir.
// It verifies every referenced object before writing any package file.
func MaterializeRelease(home, name, version, destDir string) error {
	m, err := LoadRelease(home, name, version)
	if err != nil {
		return err
	}
	contents := make(map[string][]byte, len(m.Assets))
	for _, asset := range m.Assets {
		data, err := ReadObject(home, asset.Object)
		if err != nil {
			return fmt.Errorf("read object for %s: %w", asset.Path, err)
		}
		contents[asset.Path] = data
	}
	for _, asset := range m.Assets {
		dest, err := packages.SafeJoin(destDir, asset.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, contents[asset.Path], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func releasePath(home, name, version string) (string, error) {
	if !identityPattern.MatchString(name) || !identityPattern.MatchString(version) {
		return "", fmt.Errorf("invalid installed release identity %q@%q", name, version)
	}
	return filepath.Join(config.PackageReleasesDir(home), name, version+".json"), nil
}

func contentKind(rel string) string {
	if rel == packages.ManifestFileName {
		return "manifest"
	}
	return strings.Split(rel, "/")[0]
}

func contentMediaType(rel string) string {
	switch strings.ToLower(path.Ext(rel)) {
	case ".md", ".mdc":
		return "text/markdown"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".json":
		return "application/json"
	case ".txt", ".sh", ".bash", ".zsh", ".py", ".go", ".js", ".ts", ".toml", ".ini", ".cfg", ".conf", ".xml", ".html", ".css":
		return "text/plain"
	default:
		return ""
	}
}

func validateContentManifest(m ContentManifest) error {
	if m.Schema != CurrentContentManifestSchema {
		return fmt.Errorf("declares schema %d, but this build only understands schema %d", m.Schema, CurrentContentManifestSchema)
	}
	if !identityPattern.MatchString(m.Name) {
		return fmt.Errorf("invalid package name %q", m.Name)
	}
	if !identityPattern.MatchString(m.Version) {
		return fmt.Errorf("invalid package version %q", m.Version)
	}
	if !objectIDPattern.MatchString(m.PackageDigest) {
		return fmt.Errorf("invalid package digest %q", m.PackageDigest)
	}
	if len(m.Assets) == 0 {
		return fmt.Errorf("contains no assets")
	}
	seen := make(map[string]struct{}, len(m.Assets))
	manifestFound := false
	previous := ""
	for i, asset := range m.Assets {
		if asset.Path == "" || strings.Contains(asset.Path, `\`) || path.IsAbs(asset.Path) || path.Clean(asset.Path) != asset.Path || asset.Path == "." || strings.HasPrefix(asset.Path, "../") {
			return fmt.Errorf("asset %d has unsafe or non-canonical path %q", i, asset.Path)
		}
		if _, exists := seen[asset.Path]; exists {
			return fmt.Errorf("contains duplicate asset path %q", asset.Path)
		}
		seen[asset.Path] = struct{}{}
		if previous != "" && asset.Path <= previous {
			return fmt.Errorf("asset paths are not in strictly sorted order")
		}
		previous = asset.Path
		if asset.Path == packages.ManifestFileName {
			manifestFound = true
		}
		if asset.Kind != contentKind(asset.Path) {
			return fmt.Errorf("asset %q has invalid kind %q", asset.Path, asset.Kind)
		}
		if !objectIDPattern.MatchString(string(asset.Object)) {
			return fmt.Errorf("asset %q has invalid object id %q", asset.Path, asset.Object)
		}
		if asset.Bytes < 0 {
			return fmt.Errorf("asset %q has negative byte count", asset.Path)
		}
	}
	if !manifestFound {
		return fmt.Errorf("does not reference %s", packages.ManifestFileName)
	}
	return nil
}

// ValidateContentManifest verifies that m is a safe, canonical ADR 0017
// release manifest before a registry client uses any of its paths or IDs.
func ValidateContentManifest(m ContentManifest) error {
	return validateContentManifest(m)
}

func verifyContentManifestObjects(home string, m ContentManifest) error {
	contents := make(map[string][]byte, len(m.Assets))
	for _, asset := range m.Assets {
		data, err := ReadObject(home, asset.Object)
		if err != nil {
			return fmt.Errorf("verify object for %s: %w", asset.Path, err)
		}
		if int64(len(data)) != asset.Bytes {
			return fmt.Errorf("verify object for %s: expected %d bytes, got %d", asset.Path, asset.Bytes, len(data))
		}
		contents[asset.Path] = data
	}
	h := sha256.New()
	h.Write(contents[packages.ManifestFileName])
	for _, asset := range m.Assets {
		if asset.Path == packages.ManifestFileName {
			continue
		}
		h.Write([]byte(asset.Path))
		h.Write(contents[asset.Path])
	}
	actual := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if actual != m.PackageDigest {
		return fmt.Errorf("package digest mismatch: manifest declares %s, objects hash to %s", m.PackageDigest, actual)
	}
	return nil
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
