package packages

import (
	"path/filepath"
	"testing"
)

func TestLoadWorkflowParsesFrontmatterSteps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wf-pack")
	if err := InitPackage(root, "wf-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "---\nsteps:\n  - gather\n  - review\n  - summarize\n---\n\n# Review Workflow\n")

	wf, err := LoadWorkflow(root, "review")
	if err != nil {
		t.Fatalf("LoadWorkflow() error = %v", err)
	}
	assertEqualSlices(t, wf.Steps, []string{"gather", "review", "summarize"})
	if wf.Name != "review" {
		t.Fatalf("wf.Name = %q, want %q", wf.Name, "review")
	}
}

func TestLoadWorkflowWithoutFrontmatterHasNoSteps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plain-pack")
	if err := InitPackage(root, "plain-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "# Review Workflow\n\n1. Do the thing.\n")

	wf, err := LoadWorkflow(root, "review")
	if err != nil {
		t.Fatalf("LoadWorkflow() error = %v", err)
	}
	if len(wf.Steps) != 0 {
		t.Fatalf("wf.Steps = %#v, want none for plain-prose WORKFLOW.md", wf.Steps)
	}
}

func TestLoadWorkflowRejectsUnterminatedFrontmatter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken-fm-pack")
	if err := InitPackage(root, "broken-fm-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "---\nsteps:\n  - gather\n\n# No closing delimiter\n")

	if _, err := LoadWorkflow(root, "review"); err == nil {
		t.Fatal("LoadWorkflow() error = nil, want error for unterminated frontmatter")
	}
}

func TestValidateWorkflowStepsRejectsMissingSkill(t *testing.T) {
	pkg := Package{
		Manifest: Manifest{Name: "wf-pack"},
		Skills:   []string{"gather"},
	}
	wf := Workflow{Name: "review", Steps: []string{"gather", "does-not-exist"}}

	if err := ValidateWorkflowSteps(pkg, wf); err == nil {
		t.Fatal("ValidateWorkflowSteps() error = nil, want error naming the missing step")
	}
}

func TestValidateWorkflowStepsPassesWhenAllStepsResolve(t *testing.T) {
	pkg := Package{
		Manifest: Manifest{Name: "wf-pack"},
		Skills:   []string{"gather", "review", "summarize"},
	}
	wf := Workflow{Name: "review", Steps: []string{"gather", "review"}}

	if err := ValidateWorkflowSteps(pkg, wf); err != nil {
		t.Fatalf("ValidateWorkflowSteps() error = %v, want nil", err)
	}
}

func TestFindWorkflowLocatesOwningPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owner-pack")
	if err := InitPackage(root, "owner-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "gather", "SKILL.md"), "# Gather")
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "---\nsteps:\n  - gather\n---\n")

	pkg, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	owner, wf, err := FindWorkflow([]Package{pkg}, "review")
	if err != nil {
		t.Fatalf("FindWorkflow() error = %v", err)
	}
	if owner.Manifest.Name != "owner-pack" {
		t.Fatalf("owner.Manifest.Name = %q, want %q", owner.Manifest.Name, "owner-pack")
	}
	assertEqualSlices(t, wf.Steps, []string{"gather"})
}

func TestFindWorkflowNotFound(t *testing.T) {
	if _, _, err := FindWorkflow(nil, "missing"); err == nil {
		t.Fatal("FindWorkflow() error = nil, want error for no matching workflow")
	}
}

func TestFindWorkflowAmbiguousAcrossPackages(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "pack-a")
	if err := InitPackage(rootA, "pack-a"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(rootA, "skills", "gather", "SKILL.md"), "# Gather")
	mustWrite(t, filepath.Join(rootA, "workflows", "shared-name", WorkflowFileName), "---\nsteps:\n  - gather\n---\n")

	rootB := filepath.Join(t.TempDir(), "pack-b")
	if err := InitPackage(rootB, "pack-b"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(rootB, "skills", "gather", "SKILL.md"), "# Gather")
	mustWrite(t, filepath.Join(rootB, "workflows", "shared-name", WorkflowFileName), "---\nsteps:\n  - gather\n---\n")

	pkgA, err := Discover(rootA)
	if err != nil {
		t.Fatal(err)
	}
	pkgB, err := Discover(rootB)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := FindWorkflow([]Package{pkgA, pkgB}, "shared-name"); err == nil {
		t.Fatal("FindWorkflow() error = nil, want error for a workflow name declared by two enabled packages")
	}
}

func TestFindWorkflowRejectsBrokenStepReference(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken-step-pack")
	if err := InitPackage(root, "broken-step-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "---\nsteps:\n  - does-not-exist\n---\n")

	pkg, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := FindWorkflow([]Package{pkg}, "review"); err == nil {
		t.Fatal("FindWorkflow() error = nil, want error for a step referencing a nonexistent skill")
	}
}
