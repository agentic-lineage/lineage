package packages

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"testing"
)

func TestExportIsDeterministic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "export-pack")
	if err := InitPackage(root, "export-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review")

	var first, second bytes.Buffer
	if err := Export(root, &first); err != nil {
		t.Fatalf("Export() #1 error = %v", err)
	}
	if err := Export(root, &second); err != nil {
		t.Fatalf("Export() #2 error = %v", err)
	}

	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("Export() produced different bytes for identical content across two runs")
	}
}

func TestExportRefusesWhenValidationFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken-pack")
	if err := InitPackage(root, "broken-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".env"), "API_KEY=whatever")

	var buf bytes.Buffer
	if err := Export(root, &buf); err == nil {
		t.Fatal("Export() error = nil, want error for a package that fails validation")
	}
}

func TestExportIncludesManifestAndContentSortedOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content-pack")
	if err := InitPackage(root, "content-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "zzz-last", "SKILL.md"), "# Last")
	mustWrite(t, filepath.Join(root, "skills", "aaa-first", "SKILL.md"), "# First")

	var buf bytes.Buffer
	if err := Export(root, &buf); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	names := readTarNames(t, buf.Bytes())
	if len(names) < 3 {
		t.Fatalf("archive entries = %#v, want at least manifest + 2 skill files", names)
	}
	if names[0] != ManifestFileName {
		t.Fatalf("archive entries = %#v, want manifest first", names)
	}
	firstIdx, lastIdx := -1, -1
	for i, n := range names {
		if n == filepath.ToSlash(filepath.Join("skills", "aaa-first", "SKILL.md")) {
			firstIdx = i
		}
		if n == filepath.ToSlash(filepath.Join("skills", "zzz-last", "SKILL.md")) {
			lastIdx = i
		}
	}
	if firstIdx == -1 || lastIdx == -1 || firstIdx > lastIdx {
		t.Fatalf("archive entries = %#v, want aaa-first before zzz-last", names)
	}
}

func readTarNames(t *testing.T, data []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read error = %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}
