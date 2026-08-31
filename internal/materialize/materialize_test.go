package materialize

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/packages"
	"github.com/agentic-lineage/lineage/internal/provider"
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

// TestApplyRejectsCollidingSkillDirNames covers a regression where two
// different (package, skill) pairs whose "-"-joined staged directory names
// collide (package "foo-bar" skill "x" and package "foo" skill "bar-x"
// both produce "foo-bar-x") would silently overwrite one another in a map,
// permanently dropping one package's skill from what actually gets staged.
// Apply/NeedsApproval must now fail loudly with a clear error instead.
func TestApplyRejectsCollidingSkillDirNames(t *testing.T) {
	root := t.TempDir()
	first := buildTestPackage(t, "foo-bar", "x")
	second := buildTestPackage(t, "foo", "bar-x")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	if err := Apply(root, adapter, []packages.Package{first, second}); err == nil {
		t.Fatal("Apply() error = nil, want an error for colliding skill directory names")
	}

	if _, err := NeedsApproval(root, adapter, []packages.Package{first, second}); err == nil {
		t.Fatal("NeedsApproval() error = nil, want an error for colliding skill directory names")
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

// TestApplyCapsStagedFilePermissions covers a regression where copyFile
// copied a source file's permission bits verbatim. A package could ship a
// skill file with 0o666/0o777 permissions and have that exact mode
// replicated into the receiver's project, potentially leaving
// world-writable files behind on a multi-user machine.
func TestApplyCapsStagedFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows chmod does not expose POSIX group/other write bits, so the 0o755 cap has no equivalent FileMode assertion")
	}
	withUmask0(func() {
		root := t.TempDir()
		pkg := buildTestPackage(t, "loose-perms-pack", "loose")
		skillSrc := filepath.Join(pkg.Path, "skills", "loose", "SKILL.md")
		if err := os.Chmod(skillSrc, 0o777); err != nil {
			t.Fatal(err)
		}

		claude, err := provider.Get("claude")
		if err != nil {
			t.Fatal(err)
		}
		if err := Apply(root, claude, []packages.Package{pkg}); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}

		staged := filepath.Join(root, ".claude", "skills", "loose-perms-pack-loose", "SKILL.md")
		info, err := os.Stat(staged)
		if err != nil {
			t.Fatalf("expected staged skill at %s: %v", staged, err)
		}
		assertNoLooseWriteBits(t, info.Mode())
	})
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

func TestApplyStagesSkillsAndConfigForAiderAdapter(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	aider, err := provider.Get("aider")
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(root, aider, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	skillFile := filepath.Join(root, ".aider", "skills", "review-pack-review", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("expected staged skill at %s: %v", skillFile, err)
	}

	conventions, err := os.ReadFile(filepath.Join(root, "CONVENTIONS.md"))
	if err != nil {
		t.Fatalf("read Aider conventions: %v", err)
	}
	if !containsAll(string(conventions), beginMarker, endMarker, "review-pack@0.1.0") {
		t.Fatalf("Aider conventions missing expected content:\n%s", conventions)
	}

	configData, err := os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	if err != nil {
		t.Fatalf("read Aider config: %v", err)
	}
	if !strings.Contains(string(configData), "read:") || !strings.Contains(string(configData), "CONVENTIONS.md") {
		t.Fatalf("Aider config missing conventions read entry:\n%s", configData)
	}
}

func TestAiderConfigPreservesCommentsAndSupportsScalarRead(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	original := "# keep this comment\nread: custom_rules.md\n# keep this formatting\nfoo: bar\n"
	if err := os.WriteFile(filepath.Join(root, ".aider.conf.yml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	aider, _ := provider.Get("aider")
	if err := Apply(root, aider, []packages.Package{pkg}); err != nil {
		t.Fatalf("Apply() with scalar read error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(got)
	if !containsAll(content, "# keep this comment", "# keep this formatting", "foo: bar", "custom_rules.md", "CONVENTIONS.md") {
		t.Fatalf("Aider config lost content while linking conventions:\n%s", content)
	}
}

func TestAiderConfigListDoesNotDuplicateConventions(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	original := "read:\n  - custom_rules.md\n  - CONVENTIONS.md\n"
	if err := os.WriteFile(filepath.Join(root, ".aider.conf.yml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	aider, _ := provider.Get("aider")
	if err := Apply(root, aider, []packages.Package{pkg}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	if strings.Count(string(got), "CONVENTIONS.md") != 1 {
		t.Fatalf("Aider config duplicated conventions entry:\n%s", got)
	}
}

func TestAiderConfigFlowListPreservesEntries(t *testing.T) {
	root := t.TempDir()
	original := "read: [custom_rules.md]\nfoo: bar\n"
	if err := os.WriteFile(filepath.Join(root, ".aider.conf.yml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	aider, _ := provider.Get("aider")
	state, err := aider.Config.Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	content := string(got)
	if !strings.Contains(content, "read: [custom_rules.md, CONVENTIONS.md]") || !strings.Contains(content, "foo: bar") {
		t.Fatalf("flow-list Aider config was not preserved: %s", content)
	}
	if err := aider.Config.Remove(root, state); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	if string(got) != original {
		t.Fatalf("flow-list Aider config was not restored: got %q want %q", got, original)
	}
}

func TestAiderConfigQuotedListDoesNotDuplicateConventions(t *testing.T) {
	root := t.TempDir()
	original := "read:\n  - \"CONVENTIONS.md\"\n  - custom_rules.md\n"
	if err := os.WriteFile(filepath.Join(root, ".aider.conf.yml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	aider, _ := provider.Get("aider")
	if state, err := aider.Config.Ensure(root); err != nil {
		t.Fatal(err)
	} else if len(state.Managed) != 0 {
		t.Fatalf("quoted conventions entry caused a rewrite: %#v", state)
	}
	got, _ := os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	if string(got) != original {
		t.Fatalf("quoted-list Aider config changed: got %q want %q", got, original)
	}
}

func TestAiderConfigCleanupAndApproval(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	aider, _ := provider.Get("aider")
	needs, err := NeedsApproval(root, aider, []packages.Package{pkg})
	if err != nil || !needs {
		t.Fatalf("NeedsApproval() before Aider materialization = %v, %v; want true", needs, err)
	}
	if err := Apply(root, aider, []packages.Package{pkg}); err != nil {
		t.Fatal(err)
	}
	needs, err = NeedsApproval(root, aider, []packages.Package{pkg})
	if err != nil || needs {
		t.Fatalf("NeedsApproval() after Aider materialization = %v, %v; want false", needs, err)
	}
	if err := os.Remove(filepath.Join(root, ".aider.conf.yml")); err != nil {
		t.Fatal(err)
	}
	needs, err = NeedsApproval(root, aider, []packages.Package{pkg})
	if err != nil || !needs {
		t.Fatalf("NeedsApproval() after removing Aider config = %v, %v; want true", needs, err)
	}
	if err := Apply(root, aider, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".aider.conf.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected Lineage-created Aider config cleanup, stat error = %v", err)
	}
}

func TestAiderConfigCleanupPreservesExistingConfig(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	original := "# keep\nfoo: bar\n"
	if err := os.WriteFile(filepath.Join(root, ".aider.conf.yml"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	aider, _ := provider.Get("aider")
	if err := Apply(root, aider, []packages.Package{pkg}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, aider, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, ".aider.conf.yml"))
	if string(got) != original {
		t.Fatalf("existing Aider config was not restored:\n got %q\nwant %q", got, original)
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

func TestApplyWritesCurrentSchema(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, claude, []packages.Package{pkg}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(statePath(root, "claude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema": 1`) {
		t.Fatalf("materialized state = %s, want a schema field set to %d", data, currentStateSchema)
	}
}

func TestNeedsApprovalDefaultsMissingSchemaToCurrent(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	path := statePath(root, "claude")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(adapter.SkillsDir, pkg.Manifest.Name+"-"+pkg.Skills[0])
	legacy := `{"skill_dirs":["` + filepath.ToSlash(rel) + `"]}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	needs, err := NeedsApproval(root, adapter, []packages.Package{pkg})
	if err != nil {
		t.Fatalf("NeedsApproval() error = %v", err)
	}
	if needs {
		t.Fatal("NeedsApproval() = true against a pre-schema state file that already matches, want false")
	}
}

func TestNeedsApprovalRejectsUnsupportedSchema(t *testing.T) {
	root := t.TempDir()
	pkg := buildTestPackage(t, "review-pack", "review")
	adapter := provider.Provider{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"}

	path := statePath(root, "claude")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":99,"skill_dirs":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NeedsApproval(root, adapter, []packages.Package{pkg}); err == nil {
		t.Fatal("NeedsApproval() error = nil, want error for unsupported schema")
	}
}
