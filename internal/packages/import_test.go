package packages

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestImportRoundTripDigestMatches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "roundtrip-pack")
	if err := InitPackage(root, "roundtrip-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review")
	mustWrite(t, filepath.Join(root, "references", "notes.md"), "# Notes")

	original, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	if err := Export(root, &archive); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	destParent := t.TempDir()
	name, err := Import(bytes.NewReader(archive.Bytes()), destParent, "")
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if name != "roundtrip-pack" {
		t.Fatalf("Import() name = %q, want %q", name, "roundtrip-pack")
	}

	imported, err := Discover(filepath.Join(destParent, name))
	if err != nil {
		t.Fatalf("Discover(imported) error = %v", err)
	}

	if imported.Digest != original.Digest {
		t.Fatalf("imported digest = %q, want %q (exact reconstruction)", imported.Digest, original.Digest)
	}
	assertEqualSlices(t, imported.Skills, original.Skills)
	if imported.Manifest.Name != original.Manifest.Name || imported.Manifest.Version != original.Manifest.Version {
		t.Fatalf("imported manifest = %+v, want name/version to match original %+v", imported.Manifest, original.Manifest)
	}
}

func TestImportWithAsNameOverride(t *testing.T) {
	root := filepath.Join(t.TempDir(), "original-name")
	if err := InitPackage(root, "original-name"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review")

	var archive bytes.Buffer
	if err := Export(root, &archive); err != nil {
		t.Fatal(err)
	}

	destParent := t.TempDir()
	name, err := Import(bytes.NewReader(archive.Bytes()), destParent, "renamed-pack")
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if name != "renamed-pack" {
		t.Fatalf("Import() name = %q, want %q", name, "renamed-pack")
	}
	if _, err := Discover(filepath.Join(destParent, "renamed-pack")); err != nil {
		t.Fatalf("Discover(renamed) error = %v", err)
	}
}

func TestImportRefusesExistingName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dup-pack")
	if err := InitPackage(root, "dup-pack"); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	if err := Export(root, &archive); err != nil {
		t.Fatal(err)
	}

	destParent := t.TempDir()
	if err := InitPackage(filepath.Join(destParent, "dup-pack"), "dup-pack"); err != nil {
		t.Fatal(err)
	}

	if _, err := Import(bytes.NewReader(archive.Bytes()), destParent, ""); err == nil {
		t.Fatal("Import() error = nil, want error when the destination package already exists")
	}
}

func TestImportRejectsPathTraversalEntries(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	malicious := []byte("schema: 1\nname: evil\nversion: 1.0.0\n")
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "../../etc/evil.yaml",
		Size:     int64(len(malicious)),
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(malicious); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	destParent := t.TempDir()
	if _, err := Import(bytes.NewReader(buf.Bytes()), destParent, ""); err == nil {
		t.Fatal("Import() error = nil, want error for a path-traversing archive entry")
	}
}

func TestImportRefusesArchiveThatFailsValidation(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	manifest := []byte("schema: 1\nname: leaky\nversion: 1.0.0\n")
	writeTarEntry(t, tw, ManifestFileName, manifest)
	writeTarEntry(t, tw, filepath.Join("references", ".env"), []byte("SECRET=1"))

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	destParent := t.TempDir()
	if _, err := Import(bytes.NewReader(buf.Bytes()), destParent, ""); err == nil {
		t.Fatal("Import() error = nil, want error for an archive whose content fails validation")
	}
	entries, _ := os.ReadDir(destParent)
	if len(entries) != 0 {
		t.Fatalf("destParent has %d entries, want nothing left behind after a failed import", len(entries))
	}
}

func writeTarEntry(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     filepath.ToSlash(name),
		Size:     int64(len(data)),
		Mode:     0o644,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}
