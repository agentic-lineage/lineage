package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lineage-dev/lineage/internal/packages"
	"github.com/lineage-dev/lineage/internal/provider"
)

func buildWorkflowPackage(t *testing.T, name string, skills ...string) packages.Package {
	t.Helper()
	pkgDir := filepath.Join(t.TempDir(), name)
	if err := packages.InitPackage(pkgDir, name); err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		mustWriteMaterialize(t, filepath.Join(pkgDir, "skills", skill, "SKILL.md"), "# "+skill)
	}

	pkg, err := packages.Discover(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestApplyWorkflowStagesOnlyItsSteps(t *testing.T) {
	root := t.TempDir()
	pkg := buildWorkflowPackage(t, "multi-pack", "gather", "review", "unused-skill")
	wf := packages.Workflow{Name: "review-flow", Steps: []string{"gather", "review"}}
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}

	if err := ApplyWorkflow(root, claude, pkg, wf); err != nil {
		t.Fatalf("ApplyWorkflow() error = %v", err)
	}

	for _, skill := range []string{"gather", "review"} {
		if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "multi-pack-"+skill)); err != nil {
			t.Fatalf("expected %s staged: %v", skill, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "multi-pack-unused-skill")); !os.IsNotExist(err) {
		t.Fatalf("unused-skill should not be staged, stat err = %v", err)
	}
}

func TestApplyWorkflowWritesOrderedSequence(t *testing.T) {
	root := t.TempDir()
	pkg := buildWorkflowPackage(t, "seq-pack", "gather", "review", "summarize")
	wf := packages.Workflow{Name: "review-flow", Steps: []string{"gather", "review", "summarize"}}
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}

	if err := ApplyWorkflow(root, claude, pkg, wf); err != nil {
		t.Fatalf("ApplyWorkflow() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !containsAll(string(content), "Active workflow: review-flow", "1. gather", "2. review", "3. summarize") {
		t.Fatalf("CLAUDE.md missing expected ordered sequence:\n%s", content)
	}
}

func TestNeedsApprovalForWorkflowScopedToSteps(t *testing.T) {
	root := t.TempDir()
	pkg := buildWorkflowPackage(t, "scoped-pack", "gather", "review")
	wf := packages.Workflow{Name: "review-flow", Steps: []string{"gather"}}
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}

	needs, err := NeedsApprovalForWorkflow(root, claude, pkg, wf)
	if err != nil {
		t.Fatalf("NeedsApprovalForWorkflow() error = %v", err)
	}
	if !needs {
		t.Fatal("NeedsApprovalForWorkflow() = false on first run, want true")
	}

	if err := ApplyWorkflow(root, claude, pkg, wf); err != nil {
		t.Fatal(err)
	}

	needs, err = NeedsApprovalForWorkflow(root, claude, pkg, wf)
	if err != nil {
		t.Fatalf("NeedsApprovalForWorkflow() error = %v", err)
	}
	if needs {
		t.Fatal("NeedsApprovalForWorkflow() = true after matching Apply, want false")
	}
}

func TestApplyWorkflowThenFullApplyRestoresWholePackage(t *testing.T) {
	root := t.TempDir()
	pkg := buildWorkflowPackage(t, "restore-pack", "gather", "review")
	wf := packages.Workflow{Name: "review-flow", Steps: []string{"gather"}}
	claude, err := provider.Get("claude")
	if err != nil {
		t.Fatal(err)
	}

	if err := ApplyWorkflow(root, claude, pkg, wf); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "restore-pack-review")); !os.IsNotExist(err) {
		t.Fatalf("review should not be staged by the workflow-scoped Apply, stat err = %v", err)
	}

	// A normal, full Apply for the same provider afterward should restore
	// the whole package's skill set, since both share the same state file
	// and Apply always reconciles to exactly the set it's given.
	if err := Apply(root, claude, []packages.Package{pkg}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "restore-pack-review")); err != nil {
		t.Fatalf("expected review restored by full Apply: %v", err)
	}
}
