package packages

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPackageInitIsIdempotentAndPreservesManifest guards against a
// regression called out by the project's own guidance (docs/testing.md,
// .agents/skills/lineage-go-engineering/SKILL.md: "running the same command
// twice should not duplicate config or corrupt files"). Re-running
// `lineage package init` against a package directory that already has a
// manifest must not silently discard hand-edited fields such as version
// and description, or any custom skill content already added.
func TestPackageInitIsIdempotentAndPreservesManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mypack")
	if err := InitPackage(dir, "mypack"); err != nil {
		t.Fatalf("InitPackage() error = %v", err)
	}

	// Simulate a maintainer hand-editing the manifest after the initial init.
	manifest, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Version = "2.5.0"
	manifest.Description = "Hand-edited description, please keep me"
	if err := SaveManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}

	// Also simulate a custom skill someone added inside the package.
	customSkill := filepath.Join(dir, "skills", "custom-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(customSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customSkill, []byte("# custom"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-running init against the same directory (e.g. by mistake, or as
	// part of a script) must not destroy the hand-edited manifest fields.
	if err := InitPackage(dir, "mypack"); err != nil {
		t.Fatalf("InitPackage() second call error = %v", err)
	}

	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "2.5.0" {
		t.Fatalf("Version = %q after re-init, want preserved %q (InitPackage overwrote an existing lineage.yaml with defaults)", got.Version, "2.5.0")
	}
	if got.Description != "Hand-edited description, please keep me" {
		t.Fatalf("Description = %q after re-init, want preserved value", got.Description)
	}
	if _, err := os.Stat(customSkill); err != nil {
		t.Fatalf("custom skill file lost after re-init: %v", err)
	}
}
