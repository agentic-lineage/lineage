package packages

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func TestExportRefusesPortabilityBlockers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blocked-export")
	if err := InitPackage(root, "blocked-export"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".env"), "API_KEY=whatever")

	var buf bytes.Buffer
	err := Export(root, &buf)
	if err == nil {
		t.Fatal("Export() error = nil, want error for unresolved portability blockers")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("unresolved portability blockers")) {
		t.Fatalf("Export() error = %v, want portability blocker message", err)
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

// TestExportImportPreservesExecutableBit covers #165: export hardcoded
// Mode 0o644 on every tar entry and import wrote every file at 0o644, so a
// helper script under adapters/ lost +x on export and never got it back.
func TestExportImportPreservesExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execute bits are not meaningful on Windows")
	}
	root := filepath.Join(t.TempDir(), "exec-pack")
	if err := InitPackage(root, "exec-pack"); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "adapters", "run.sh")
	mustWrite(t, script, "#!/bin/sh\necho hi\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(root, "references", "notes.md")
	mustWrite(t, plain, "# notes\n")

	var buf bytes.Buffer
	if err := Export(root, &buf); err != nil {
		t.Fatal(err)
	}

	destParent := t.TempDir()
	name, err := Import(&buf, destParent, "")
	if err != nil {
		t.Fatal(err)
	}
	imported := filepath.Join(destParent, name)

	gotScript, err := os.Stat(filepath.Join(imported, "adapters", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if gotScript.Mode().Perm() != 0o755 {
		t.Errorf("adapters/run.sh mode = %o after a round trip, want 755", gotScript.Mode().Perm())
	}

	gotPlain, err := os.Stat(filepath.Join(imported, "references", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if gotPlain.Mode().Perm() != 0o644 {
		t.Errorf("references/notes.md mode = %o after a round trip, want 644", gotPlain.Mode().Perm())
	}
}

// TestContentModeCollapsesUnsafeBits asserts that only the exec bit
// survives: package content is untrusted, so an archive must not be able to
// request setuid, setgid, sticky, or group/world-writable permissions.
func TestContentModeCollapsesUnsafeBits(t *testing.T) {
	tests := []struct {
		in   os.FileMode
		want os.FileMode
	}{
		{0o644, 0o644},
		{0o600, 0o644},
		{0o755, 0o755},
		{0o700, 0o755},
		{0o777, 0o755},
		{0o666, 0o644},
		{os.FileMode(0o4755) | os.ModeSetuid, 0o755},
		{os.FileMode(0o2644) | os.ModeSetgid, 0o644},
	}
	for _, tt := range tests {
		if got := ContentMode(tt.in); got != tt.want {
			t.Errorf("ContentMode(%o) = %o, want %o", tt.in, got, tt.want)
		}
	}
}
