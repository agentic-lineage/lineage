package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildTestPackage(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := packages.InitPackage(dir, name); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "skills", "hello", "SKILL.md"), "# Hello\n")
	mustWrite(t, filepath.Join(dir, "skills", "world", "SKILL.md"), "# World\n")
	return dir
}

func TestWriteObjectSameContentSameID(t *testing.T) {
	home := t.TempDir()

	id1, err := WriteObject(home, []byte("hello"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	id2, err := WriteObject(home, []byte("hello"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	if id1 != id2 {
		t.Fatalf("WriteObject() ids = %q, %q for identical content, want equal", id1, id2)
	}
}

func TestWriteObjectDifferentContentDifferentID(t *testing.T) {
	home := t.TempDir()

	id1, err := WriteObject(home, []byte("hello"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	id2, err := WriteObject(home, []byte("goodbye"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	if id1 == id2 {
		t.Fatalf("WriteObject() ids = %q for different content, want distinct", id1)
	}
}

func TestReadObjectRoundTrips(t *testing.T) {
	home := t.TempDir()

	id, err := WriteObject(home, []byte("payload"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	data, err := ReadObject(home, id)
	if err != nil {
		t.Fatalf("ReadObject() error = %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("ReadObject() = %q, want %q", data, "payload")
	}
}

func TestReadObjectDetectsCorruption(t *testing.T) {
	home := t.TempDir()

	id, err := WriteObject(home, []byte("payload"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	path, err := blobPath(config.ObjectsDir(home), id)
	if err != nil {
		t.Fatalf("blobPath() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadObject(home, id); err == nil {
		t.Fatal("ReadObject() error = nil for tampered object, want error")
	}
}

func TestVerifyObject(t *testing.T) {
	home := t.TempDir()

	id, err := WriteObject(home, []byte("payload"))
	if err != nil {
		t.Fatalf("WriteObject() error = %v", err)
	}
	if err := VerifyObject(home, id); err != nil {
		t.Fatalf("VerifyObject() error = %v for untampered object", err)
	}

	path, err := blobPath(config.ObjectsDir(home), id)
	if err != nil {
		t.Fatalf("blobPath() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyObject(home, id); err == nil {
		t.Fatal("VerifyObject() error = nil for tampered object, want error")
	}
}

func TestCreateIsDeterministic(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	_, id1, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, id2, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id1 != id2 {
		t.Fatalf("Create() ids = %q, %q for unchanged content, want equal", id1, id2)
	}
}

func TestCreateChangesIDWhenContentChanges(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	_, id1, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	mustWrite(t, filepath.Join(dir, "skills", "hello", "SKILL.md"), "# Hello, changed\n")

	_, id2, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id1 == id2 {
		t.Fatal("Create() id unchanged after content changed, want different id")
	}
}

func TestCreateDedupesIdenticalFileContent(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")
	// Both skills/hello/SKILL.md and skills/world/SKILL.md exist; make them
	// byte-identical so their objects should collapse to one ID.
	mustWrite(t, filepath.Join(dir, "skills", "hello", "SKILL.md"), "# Same\n")
	mustWrite(t, filepath.Join(dir, "skills", "world", "SKILL.md"), "# Same\n")

	m, _, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var helloID, worldID ObjectID
	for _, f := range m.Files {
		switch f.Path {
		case "skills/hello/SKILL.md":
			helloID = f.Object
		case "skills/world/SKILL.md":
			worldID = f.Object
		}
	}
	if helloID == "" || worldID == "" {
		t.Fatalf("Create() manifest missing expected files: %+v", m.Files)
	}
	if helloID != worldID {
		t.Fatalf("Create() object ids = %q, %q for byte-identical files, want equal (dedup)", helloID, worldID)
	}
}

func TestCreateManifestIncludesManifestFile(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	m, _, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	found := false
	for _, f := range m.Files {
		if f.Path == "lineage.yaml" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Create() manifest files = %+v, want lineage.yaml included", m.Files)
	}
	if m.Name != "agent-pack" || m.Version != "0.1.0" {
		t.Fatalf("Create() manifest name/version = %q/%q, want agent-pack/0.1.0", m.Name, m.Version)
	}
}

func TestLoadManifestRoundTrips(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	created, id, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	loaded, err := LoadManifest(home, id)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if loaded.Name != created.Name || loaded.Version != created.Version || len(loaded.Files) != len(created.Files) {
		t.Fatalf("LoadManifest() = %+v, want %+v", loaded, created)
	}
}

func TestLoadManifestDetectsCorruption(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	_, id, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	path, err := blobPath(config.SnapshotsDir(home), id)
	if err != nil {
		t.Fatalf("blobPath() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not the real manifest"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadManifest(home, id); err == nil {
		t.Fatal("LoadManifest() error = nil for tampered manifest, want error")
	}
}

func TestMaterializeReconstructsPackage(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	m, _, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "restored")
	if err := Materialize(home, m, destDir); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}

	for _, rel := range []string{"lineage.yaml", "skills/hello/SKILL.md", "skills/world/SKILL.md"} {
		orig, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read original %s: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(destDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read materialized %s: %v", rel, err)
		}
		if string(got) != string(orig) {
			t.Fatalf("materialized %s = %q, want %q", rel, got, orig)
		}
	}
}

func TestMaterializeDetectsCorruptObjectBeforeWriting(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	m, _, err := Create(home, dir)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Corrupt the object backing one file.
	var target ObjectID
	for _, f := range m.Files {
		if f.Path == "skills/hello/SKILL.md" {
			target = f.Object
		}
	}
	if target == "" {
		t.Fatal("expected skills/hello/SKILL.md in manifest")
	}
	path, err := blobPath(config.ObjectsDir(home), target)
	if err != nil {
		t.Fatalf("blobPath() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(t.TempDir(), "restored")
	if err := Materialize(home, m, destDir); err == nil {
		t.Fatal("Materialize() error = nil with a corrupt object, want error")
	}
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Fatalf("Materialize() wrote to destDir despite a corrupt object; destDir stat err = %v, want IsNotExist", err)
	}
}

func TestAllManifestIDsEmptyBeforeAnyCreate(t *testing.T) {
	home := t.TempDir()
	ids, err := AllManifestIDs(home)
	if err != nil {
		t.Fatalf("AllManifestIDs() error = %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("AllManifestIDs() = %v, want none before any Create call", ids)
	}
}

func TestAllManifestIDsListsEveryCreatedSnapshot(t *testing.T) {
	home := t.TempDir()
	_, id1, err := Create(home, buildTestPackage(t, "agent-pack"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, id2, err := Create(home, buildTestPackage(t, "other-pack"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	ids, err := AllManifestIDs(home)
	if err != nil {
		t.Fatalf("AllManifestIDs() error = %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("AllManifestIDs() = %v, want exactly the 2 created manifests", ids)
	}
	found := map[ObjectID]bool{ids[0]: true, ids[1]: true}
	if !found[id1] || !found[id2] {
		t.Fatalf("AllManifestIDs() = %v, want it to include %s and %s", ids, id1, id2)
	}
}

func TestAllManifestIDsDedupesIdenticalSnapshots(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")
	if _, _, err := Create(home, dir); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, _, err := Create(home, dir); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	ids, err := AllManifestIDs(home)
	if err != nil {
		t.Fatalf("AllManifestIDs() error = %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("AllManifestIDs() = %v, want identical content deduped to one manifest", ids)
	}
}
