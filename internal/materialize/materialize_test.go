package materialize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lineage-dev/lineage/internal/packages"
	"github.com/lineage-dev/lineage/internal/provider"
)

func TestNeedsApprovalOnFirstRun(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	needs, err := NeedsApproval(root, adapter, []packages.Package{pkg})
	if err != nil {
		t.Fatalf("NeedsApproval() error = %v", err)
	}
	if !needs {
		t.Fatal("NeedsApproval() = false on first run, want true")
	}
}

func TestNeedsApprovalFalseAfterMatchingApply(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	if err := Apply(root, adapter, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	needs, err := NeedsApproval(root, adapter, []packages.Package{pkg})
	if err != nil {
		t.Fatalf("NeedsApproval() error = %v", err)
	}
	if needs {
		t.Fatal("NeedsApproval() = true for an already-materialized, unchanged package set, want false")
	}
}

func TestNeedsApprovalTrueWhenPackageSetChanges(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	if err := Apply(root, adapter, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	other := buildTestPackage(t, "other-pack", "other-skill")
	needs, err := NeedsApproval(root, adapter, []packages.Package{pkg, other})
	if err != nil {
		t.Fatalf("NeedsApproval() error = %v", err)
	}
	if !needs {
		t.Fatal("NeedsApproval() = false after adding a new package, want true")
	}
}

func TestApplyStagesSkillsAndWritesContextFile(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")

	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, claude, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	skillFile := filepath.Join(root, ".claude", "skills", "review-pack-review", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("expected staged skill at %s: %v", skillFile, err)
	}

	contextData, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	content := string(contextData)
	if !containsAll(content, beginMarker, endMarker, "review-pack@0.1.0") {
		t.Fatalf("CLAUDE.md missing expected content:\n%s", content)
	}
}

func TestApplyStagesSkillsForCodexAdapter(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	codex, err := provider.Get("codex")
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(root, codex, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	skillFile := filepath.Join(root, ".agents", "skills", "review-pack-review", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("expected staged skill at %s: %v", skillFile, err)
	}

	contextData, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	content := string(contextData)
	if !containsAll(content, beginMarker, endMarker, "review-pack@0.1.0") {
		t.Fatalf("AGENTS.md missing expected content:\n%s", content)
	}
}

func TestApplyKeepsProvidersIsolated(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	codex, err := provider.Get("codex")
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(root, claude, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply(claude) error = %v", err)
	}
	if err := Apply(root, codex, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply(codex) error = %v", err)
	}

	// Disabling everything for one provider must not touch the other
	// provider's staged content or state.
	if err := Apply(root, codex, nil); err != nil {
		t.Fatalf("Apply(codex, none) error = %v", err)
	}

	claudeSkill := filepath.Join(root, ".claude", "skills", "review-pack-review")
	if _, err := os.Stat(claudeSkill); err != nil {
		t.Fatalf("expected claude skill to remain staged: %v", err)
	}
	codexSkill := filepath.Join(root, ".agents", "skills", "review-pack-review")
	if _, err := os.Stat(codexSkill); !os.IsNotExist(err) {
		t.Fatalf("expected codex skill removed, stat err = %v", err)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	if err := Apply(root, adapter, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() #1 error = %v", err)
	}
	first, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}

	if err := Apply(root, adapter, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() #2 error = %v", err)
	}
	second, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("CLAUDE.md changed between identical Apply calls:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	entries, err := os.ReadDir(filepath.Join(root, ".claude", "skills"))
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("skills dir has %d entries, want 1 (no duplication): %v", len(entries), entries)
	}
}

func TestApplyRemovesStaleSkillsWhenPackageDisabled(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	if err := Apply(root, adapter, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() #1 error = %v", err)
	}
	stagedDir := filepath.Join(root, ".claude", "skills", "review-pack-review")
	if _, err := os.Stat(stagedDir); err != nil {
		t.Fatalf("expected staged skill: %v", err)
	}

	if err := Apply(root, adapter, nil); err != nil {
		t.Fatalf("Apply() #2 error = %v", err)
	}
	if _, err := os.Stat(stagedDir); !os.IsNotExist(err) {
		t.Fatalf("expected stale skill dir removed, stat err = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !containsAll(string(content), "none") {
		t.Fatalf("expected CLAUDE.md to report no active packages, got:\n%s", content)
	}
}

func TestApplyPreservesUserContentAroundMarkers(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	mustWriteMaterialize(t, filepath.Join(root, "CLAUDE.md"), "# My project notes\n\nHand-written content.\n")

	if err := Apply(root, adapter, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !containsAll(string(content), "Hand-written content.", beginMarker, endMarker) {
		t.Fatalf("expected hand-written content preserved alongside generated block, got:\n%s", content)
	}
}

func TestApplyRefusesSymlinkedSkillContent(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")

	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(pkg.Path, "skills", "review", "link.txt")); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}
	if err := Apply(root, adapter, []packages.Package{pkg}); err == nil {
		t.Fatal("Apply() error = nil, want error for symlinked package content")
	}
}

func buildTestPackage(t *testing.T, name, skill string) packages.Package {
	t.Helper()
	pkgDir := filepath.Join(t.TempDir(), name)
	if err := packages.InitPackage(pkgDir, name); err != nil {
		t.Fatal(err)
	}
	mustWriteMaterialize(t, filepath.Join(pkgDir, "skills", skill, "SKILL.md"), "# "+skill)

	pkg, err := packages.Discover(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func mustWriteMaterialize(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func TestHasStateFalseBeforeApply(t *testing.T) {
	root := t.TempDir()
	has, err := HasState(root, "claude")
	if err != nil {
		t.Fatalf("HasState() error = %v", err)
	}
	if has {
		t.Fatal("HasState() = true before any Apply call, want false")
	}
}

func TestHasStateTrueAfterApply(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, claude, []packages.Package{pkg}); err != nil {
		t.Fatal(err)
	}

	has, err := HasState(root, "claude")
	if err != nil {
		t.Fatalf("HasState() error = %v", err)
	}
	if !has {
		t.Fatal("HasState() = false after Apply, want true")
	}

	has, err = HasState(root, "codex")
	if err != nil {
		t.Fatalf("HasState() error = %v", err)
	}
	if has {
		t.Fatal("HasState(codex) = true, want false - only claude was ever applied")
	}
}
