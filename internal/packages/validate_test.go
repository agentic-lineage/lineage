package packages

import (
	"path/filepath"
	"testing"
)

func TestValidatePassesCleanPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clean-pack")
	if err := InitPackage(root, "clean-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review")

	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", report.Errors)
	}
	if report.Digest == "" {
		t.Fatal("report.Digest is empty")
	}
}

func TestValidateCollectsMultipleErrorsNotJustFirst(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken-pack")
	if err := InitPackage(root, "broken-pack"); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Exports.Agents = []string{"missing-agent.md"}
	manifest.Exports.Workflows = []string{"missing-workflow"}
	if err := SaveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".env"), "API_KEY=whatever")

	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if report.Passed() {
		t.Fatal("report.Passed() = true, want false")
	}
	if len(report.Errors) < 3 {
		t.Fatalf("report.Errors = %#v, want at least 3 distinct failures collected in one pass", report.Errors)
	}
}

func TestValidateRejectsTraversingEntrypoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "traversal-pack")
	if err := InitPackage(root, "traversal-pack"); err != nil {
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

	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if report.Passed() {
		t.Fatal("report.Passed() = true, want false for a traversing entrypoint")
	}
}

func TestValidateNotesUnsatisfiedRequiredSkillWithoutFailing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dependent-pack")
	if err := InitPackage(root, "dependent-pack"); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Requires.Skills = []string{"security-basics"}
	if err := SaveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}

	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, want true (unsatisfied requires.skills is a note, not an error): %#v", report.Errors)
	}
	if len(report.Notes) != 1 {
		t.Fatalf("report.Notes = %#v, want one note about the unsatisfied dependency", report.Notes)
	}
}

func TestValidateFailsForUnloadableManifest(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ManifestFileName), "schema: 99\nname: future\nversion: 1.0.0\n")

	if _, err := Validate(root); err == nil {
		t.Fatal("Validate() error = nil, want error for an unsupported schema")
	}
}

func TestValidateRejectsWorkflowWithBrokenStep(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken-workflow-pack")
	if err := InitPackage(root, "broken-workflow-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "---\nsteps:\n  - does-not-exist\n---\n")

	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if report.Passed() {
		t.Fatal("report.Passed() = true, want false for a workflow step referencing a nonexistent skill")
	}
}

func TestValidatePassesWorkflowWithResolvedSteps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "good-workflow-pack")
	if err := InitPackage(root, "good-workflow-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "gather", "SKILL.md"), "# Gather")
	mustWrite(t, filepath.Join(root, "workflows", "review", WorkflowFileName), "---\nsteps:\n  - gather\n---\n")

	report, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", report.Errors)
	}
}
