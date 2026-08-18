package packages

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinAllowsPathsInsideRoot(t *testing.T) {
	root := t.TempDir()
	got, err := SafeJoin(root, filepath.Join("skills", "review", "SKILL.md"))
	if err != nil {
		t.Fatalf("SafeJoin() error = %v", err)
	}
	want := filepath.Join(root, "skills", "review", "SKILL.md")
	if got != want {
		t.Fatalf("SafeJoin() = %q, want %q", got, want)
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"../outside.txt",
		"../../etc/passwd",
		filepath.Join("skills", "..", "..", "escape.txt"),
	}
	for _, rel := range cases {
		if _, err := SafeJoin(root, rel); err == nil {
			t.Fatalf("SafeJoin(%q) error = nil, want error", rel)
		}
	}
}

func TestSafeJoinRejectsAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	if _, err := SafeJoin(root, "/etc/passwd"); err == nil {
		t.Fatal("SafeJoin() error = nil, want error for absolute path")
	}
}

func TestSafeJoinRejectsEmptyPath(t *testing.T) {
	if _, err := SafeJoin(t.TempDir(), ""); err == nil {
		t.Fatal("SafeJoin() error = nil, want error for empty path")
	}
}

func TestDiscoverRejectsTraversingEntrypoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "malicious-pack")
	if err := InitPackage(root, "malicious-pack"); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Entrypoints.Claude = "../../etc/passwd"
	if err := SaveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}

	if _, err := Discover(root); err == nil {
		t.Fatal("Discover() error = nil, want error for traversing entrypoint")
	}
}

func TestDiscoverAllowsInBoundsEntrypoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "well-behaved-pack")
	if err := InitPackage(root, "well-behaved-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "adapters", "claude", "entry.md"), "# Entry")

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Entrypoints.Claude = filepath.Join("adapters", "claude", "entry.md")
	if err := SaveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}

	if _, err := Discover(root); err != nil {
		t.Fatalf("Discover() error = %v, want no error for in-bounds entrypoint", err)
	}
}
