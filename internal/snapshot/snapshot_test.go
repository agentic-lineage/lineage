package snapshot

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
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

func TestWriteObjectCompressesOnlyWhenWorthwhile(t *testing.T) {
	t.Run("compressible", func(t *testing.T) {
		home := t.TempDir()
		data := bytes.Repeat([]byte("workflow-step: review\n"), 256)
		id, err := WriteObject(home, data)
		if err != nil {
			t.Fatal(err)
		}

		status, info, err := ObjectStorageInfo(home, id)
		if err != nil || status != ObjectVerified {
			t.Fatalf("ObjectStorageInfo() = %v, %+v, %v; want verified zstd object", status, info, err)
		}
		if info.Encoding != "zstd" || info.StoredBytes >= info.LogicalBytes {
			t.Fatalf("ObjectStorageInfo() = %+v, want smaller zstd representation", info)
		}
		rawPath, err := blobPath(config.ObjectsDir(home), id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(rawPath); !os.IsNotExist(err) {
			t.Fatalf("raw object path stat error = %v, want not exist", err)
		}
		got, err := ReadObject(home, id)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Fatal("ReadObject() did not restore the original bytes")
		}
	})

	t.Run("incompressible", func(t *testing.T) {
		home := t.TempDir()
		data := make([]byte, 256)
		for i := range data {
			data[i] = byte(i)
		}
		id, err := WriteObject(home, data)
		if err != nil {
			t.Fatal(err)
		}
		status, info, err := ObjectStorageInfo(home, id)
		if err != nil || status != ObjectVerified {
			t.Fatalf("ObjectStorageInfo() = %v, %+v, %v; want verified raw object", status, info, err)
		}
		if info.Encoding != "raw" || info.StoredBytes != int64(len(data)) {
			t.Fatalf("ObjectStorageInfo() = %+v, want raw representation", info)
		}
	})
}

func TestReadObjectDetectsCompressedCorruption(t *testing.T) {
	home := t.TempDir()
	data := bytes.Repeat([]byte("compress me\n"), 256)
	id, err := WriteObject(home, data)
	if err != nil {
		t.Fatal(err)
	}
	path, err := objectRepresentationPath(config.ObjectsDir(home), id, "zstd")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a zstd frame"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadObject(home, id); err == nil {
		t.Fatal("ReadObject() error = nil for corrupt compressed object, want error")
	}
	status, err := ObjectAvailability(home, id)
	if err == nil || status != ObjectCorrupt {
		t.Fatalf("ObjectAvailability() = %v, %v; want ObjectCorrupt, error", status, err)
	}
}

func TestInspectWeightSeparatesLogicalWeightFromLocalCAS(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "weight-pack")
	if err := packages.InitPackage(dir, "weight-pack"); err != nil {
		t.Fatal(err)
	}
	// These two assets deliberately share an object. Logical package weight
	// counts both paths, while local CAS accounting counts their body once.
	mustWrite(t, filepath.Join(dir, "skills", "one", "SKILL.md"), "same")
	mustWrite(t, filepath.Join(dir, "skills", "two", "SKILL.md"), "same")
	mustWrite(t, filepath.Join(dir, "references", "diagram.bin"), "\x00\x01")
	manifest, err := BuildContentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	for _, asset := range manifest.Assets {
		if asset.Path == "skills/one/SKILL.md" {
			if _, err := WriteObject(home, []byte("same")); err != nil {
				t.Fatal(err)
			}
		}
	}
	report, err := InspectWeight(home, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if report.Estimator.Name != "bytes-per-token" || report.Estimator.Version != "v1" || report.Estimator.Exact {
		t.Errorf("Estimator = %+v, want named non-exact v1 estimator", report.Estimator)
	}
	if report.StoredBytes <= report.LocalStorage.VerifiedBytes {
		t.Errorf("logical stored bytes %d, want greater than deduplicated verified bytes %d", report.StoredBytes, report.LocalStorage.VerifiedBytes)
	}
	if report.Stub.Bytes == 0 || !report.Stub.Context.Available || report.Stub.Context.Tokens == 0 {
		t.Errorf("stub = %+v, want exact bytes and a deterministic estimate", report.Stub)
	}
	var binary AssetWeight
	for _, asset := range report.Assets {
		if asset.Path == "references/diagram.bin" {
			binary = asset
		}
	}
	if binary.Context.Available || binary.Context.Reason == "" {
		t.Errorf("binary context = %+v, want explicit unavailable estimate", binary.Context)
	}
	if report.FullBody.Context.Available {
		t.Errorf("full body context = %+v, want unavailable when it includes a binary asset", report.FullBody.Context)
	}
}

func TestInspectWeightReportsCompressedPhysicalStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "compressed-weight-pack")
	if err := packages.InitPackage(dir, "compressed-weight-pack"); err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("repeatable workflow guidance\n"), 256)
	mustWrite(t, filepath.Join(dir, "skills", "review", "SKILL.md"), string(content))
	home := t.TempDir()
	manifest, err := StorePackage(home, dir)
	if err != nil {
		t.Fatal(err)
	}
	report, err := InspectWeight(home, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if report.LocalStorage.PhysicalBytes >= report.LocalStorage.VerifiedBytes {
		t.Fatalf("physical bytes %d, want less than verified logical bytes %d", report.LocalStorage.PhysicalBytes, report.LocalStorage.VerifiedBytes)
	}
	if report.LocalStorage.SavedBytes != report.LocalStorage.VerifiedBytes-report.LocalStorage.PhysicalBytes {
		t.Fatalf("saved bytes %d, want logical minus physical", report.LocalStorage.SavedBytes)
	}
	for _, asset := range report.Assets {
		if asset.Path == "skills/review/SKILL.md" {
			if asset.LocalEncoding != "zstd" || asset.LocalStoredBytes >= asset.Bytes {
				t.Fatalf("compressed asset = %+v, want smaller zstd representation", asset)
			}
			return
		}
	}
	t.Fatal("weight report is missing compressed skill asset")
}

func TestInspectWeightEstimatesKnownTextExtensionsAndEmptyBody(t *testing.T) {
	t.Run("known text extensions", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "text-pack")
		if err := packages.InitPackage(dir, "text-pack"); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(dir, "references", "notes.txt"), "plain text")
		mustWrite(t, filepath.Join(dir, "references", "README.MD"), "upper case markdown")
		manifest, err := BuildContentManifest(dir)
		if err != nil {
			t.Fatal(err)
		}
		report, err := InspectWeight(t.TempDir(), manifest)
		if err != nil {
			t.Fatal(err)
		}
		if !report.FullBody.Context.Available || report.FullBody.Context.Tokens == 0 {
			t.Errorf("full body context = %+v, want available estimate for text assets", report.FullBody.Context)
		}
	})

	t.Run("manifest only", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "empty-pack")
		if err := packages.InitPackage(dir, "empty-pack"); err != nil {
			t.Fatal(err)
		}
		manifest, err := BuildContentManifest(dir)
		if err != nil {
			t.Fatal(err)
		}
		report, err := InspectWeight(t.TempDir(), manifest)
		if err != nil {
			t.Fatal(err)
		}
		if !report.FullBody.Context.Available || report.FullBody.Context.Tokens != 0 {
			t.Errorf("empty full body context = %+v, want available zero estimate", report.FullBody.Context)
		}
	})
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

func TestWriteObjectRejectsCorruptExistingObject(t *testing.T) {
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
	if _, err := WriteObject(home, []byte("payload")); err == nil {
		t.Fatal("WriteObject() error = nil for corrupt existing object, want error")
	}
}

func TestWriteObjectConcurrentCallsConverge(t *testing.T) {
	home := t.TempDir()
	const writers = 16
	ids := make(chan ObjectID, writers)
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := WriteObject(home, []byte("shared payload"))
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Errorf("WriteObject() concurrent error = %v", err)
	}
	var first ObjectID
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Errorf("WriteObject() ids = %q, %q; want one shared object", first, id)
		}
	}
	if _, err := ReadObject(home, first); err != nil {
		t.Fatalf("ReadObject() after concurrent writes error = %v", err)
	}
}

func TestObjectAvailabilityDistinguishesMissingAndCorrupt(t *testing.T) {
	home := t.TempDir()
	missing := ObjectID("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	status, err := ObjectAvailability(home, missing)
	if err != nil || status != ObjectMissing {
		t.Fatalf("ObjectAvailability(missing) = %v, %v; want ObjectMissing, nil", status, err)
	}
	id, err := WriteObject(home, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	status, err = ObjectAvailability(home, id)
	if err != nil || status != ObjectVerified {
		t.Fatalf("ObjectAvailability(verified) = %v, %v; want ObjectVerified, nil", status, err)
	}
	path, err := blobPath(config.ObjectsDir(home), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = ObjectAvailability(home, id)
	if err == nil || status != ObjectCorrupt {
		t.Fatalf("ObjectAvailability(corrupt) = %v, %v; want ObjectCorrupt, error", status, err)
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

func TestMigrateLegacyInstallCreatesCompleteRelease(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")

	m, err := MigrateLegacyInstall(home, dir)
	if err != nil {
		t.Fatalf("MigrateLegacyInstall() error = %v", err)
	}
	loaded, err := LoadRelease(home, m.Name, m.Version)
	if err != nil {
		t.Fatalf("LoadRelease() error = %v", err)
	}
	if loaded.PackageDigest != m.PackageDigest || len(loaded.Assets) != len(m.Assets) {
		t.Fatalf("LoadRelease() = %+v, want %+v", loaded, m)
	}
	if _, err := os.Stat(filepath.Join(dir, packages.ManifestFileName)); err != nil {
		t.Fatalf("MigrateLegacyInstall() changed legacy package directory: %v", err)
	}
}

func TestMaterializeReleaseReconstructsVerifiedPackage(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")
	m, err := MigrateLegacyInstall(home, dir)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if err := MaterializeRelease(home, m.Name, m.Version, dest); err != nil {
		t.Fatalf("MaterializeRelease() error = %v", err)
	}
	for _, rel := range []string{"lineage.yaml", "skills/hello/SKILL.md", "skills/world/SKILL.md"} {
		want, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("materialized %s = %q, want %q", rel, got, want)
		}
	}
}

func TestStorePackageDeduplicatesAcrossPackages(t *testing.T) {
	home := t.TempDir()
	firstDir := buildTestPackage(t, "first-pack")
	secondDir := buildTestPackage(t, "second-pack")
	mustWrite(t, filepath.Join(firstDir, "skills", "hello", "SKILL.md"), "# Shared\n")
	mustWrite(t, filepath.Join(secondDir, "skills", "hello", "SKILL.md"), "# Shared\n")

	first, err := StorePackage(home, firstDir)
	if err != nil {
		t.Fatalf("StorePackage(first) error = %v", err)
	}
	second, err := StorePackage(home, secondDir)
	if err != nil {
		t.Fatalf("StorePackage(second) error = %v", err)
	}
	var firstID, secondID ObjectID
	for _, asset := range first.Assets {
		if asset.Path == "skills/hello/SKILL.md" {
			firstID = asset.Object
		}
	}
	for _, asset := range second.Assets {
		if asset.Path == "skills/hello/SKILL.md" {
			secondID = asset.Object
		}
	}
	if firstID == "" || secondID == "" || firstID != secondID {
		t.Fatalf("shared asset ids = %q, %q; want the same object", firstID, secondID)
	}
}

func TestBuildContentManifestRejectsSymlinkedPackageManifest(t *testing.T) {
	dir := buildTestPackage(t, "agent-pack")
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	mustWrite(t, outside, "schema: 1\nname: agent-pack\nversion: 0.1.0\n")
	manifestPath := filepath.Join(dir, packages.ManifestFileName)
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, manifestPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := BuildContentManifest(dir); err == nil {
		t.Fatal("BuildContentManifest() error = nil for a symlinked package manifest, want error")
	}
}

func TestCommitReleaseDoesNotPublishIncompleteManifest(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")
	m, err := BuildContentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRelease(home, m); err == nil {
		t.Fatal("CommitRelease() error = nil for missing objects, want error")
	}
	if _, err := LoadRelease(home, m.Name, m.Version); !os.IsNotExist(err) {
		t.Fatalf("LoadRelease() error = %v after failed commit, want IsNotExist", err)
	}
}

func TestCommitReleaseDoesNotReplaceExistingCompleteRelease(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")
	complete, err := MigrateLegacyInstall(home, dir)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "skills", "hello", "SKILL.md"), "# Changed\n")
	incomplete, err := BuildContentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitRelease(home, incomplete); err == nil {
		t.Fatal("CommitRelease() error = nil for an incomplete update, want error")
	}
	loaded, err := LoadRelease(home, complete.Name, complete.Version)
	if err != nil {
		t.Fatalf("LoadRelease() error = %v after incomplete update", err)
	}
	if loaded.PackageDigest != complete.PackageDigest {
		t.Fatalf("LoadRelease() package digest = %q, want previous complete digest %q", loaded.PackageDigest, complete.PackageDigest)
	}
}

func TestCommitReleaseRejectsMismatchedPackageDigest(t *testing.T) {
	home := t.TempDir()
	dir := buildTestPackage(t, "agent-pack")
	m, err := StorePackage(home, dir)
	if err != nil {
		t.Fatal(err)
	}
	m.PackageDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := CommitRelease(home, m); err == nil {
		t.Fatal("CommitRelease() error = nil for a mismatched package digest, want error")
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

func TestLoadManifestRejectsStructurallyInvalidSelfHashedManifest(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "null", data: `null`},
		{name: "unsupported schema", data: `{"schema":99,"name":"agent-pack","version":"0.1.0","files":[]}`},
		{name: "explicit zero schema", data: `{"schema":0,"name":"agent-pack","version":"0.1.0","files":[]}`},
		{name: "missing identity", data: `{"schema":1,"files":[{"path":"lineage.yaml","object":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`},
		{name: "empty files", data: `{"schema":1,"name":"agent-pack","version":"0.1.0","files":[]}`},
		{name: "missing package manifest", data: `{"schema":1,"name":"agent-pack","version":"0.1.0","files":[{"path":"skills/x/SKILL.md","object":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`},
		{name: "invalid object id", data: `{"schema":1,"name":"agent-pack","version":"0.1.0","files":[{"path":"lineage.yaml","object":"sha256:not-a-hash"}]}`},
		{name: "path traversal", data: `{"schema":1,"name":"agent-pack","version":"0.1.0","files":[{"path":"../lineage.yaml","object":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`},
		{name: "duplicate path", data: `{"schema":1,"name":"agent-pack","version":"0.1.0","files":[{"path":"lineage.yaml","object":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"path":"lineage.yaml","object":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			id, err := putBlob(config.SnapshotsDir(home), []byte(tt.data))
			if err != nil {
				t.Fatalf("putBlob() error = %v", err)
			}
			if _, err := LoadManifest(home, id); err == nil {
				t.Fatalf("LoadManifest() error = nil for %s", tt.data)
			}
		})
	}
}

func TestBlobPathRejectsNonCanonicalObjectIDs(t *testing.T) {
	for _, id := range []ObjectID{
		"sha256:abc",
		"sha256:../../outside",
		"sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"md5:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		if _, err := blobPath(t.TempDir(), id); err == nil {
			t.Errorf("blobPath(%q) error = nil", id)
		}
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

func TestAllManifestIDsRejectsMalformedStoreEntries(t *testing.T) {
	home := t.TempDir()
	badPath := filepath.Join(config.SnapshotsDir(home), "not-a-hash")
	mustWrite(t, badPath, "junk")
	if _, err := AllManifestIDs(home); err == nil {
		t.Fatal("AllManifestIDs() error = nil for malformed store entry")
	}
}
