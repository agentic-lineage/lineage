package packages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanSetupReportsExistingVsMissing(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tasks.csv"), "title,owner,status\n")

	setup := Setup{
		Files: []SetupFile{
			{Path: "tasks.csv", Template: "title,owner,status\n"},
			{Path: "notes/log.md", Template: "# Log\n"},
		},
		Directories: []SetupDirectory{
			{Path: "output"},
		},
	}

	plan, err := PlanSetup(root, setup)
	if err != nil {
		t.Fatalf("PlanSetup() error = %v", err)
	}
	if len(plan.Files) != 2 || !plan.Files[0].Exists || plan.Files[1].Exists {
		t.Fatalf("plan.Files = %+v, want [tasks.csv Exists=true, notes/log.md Exists=false]", plan.Files)
	}
	if len(plan.Directories) != 1 || plan.Directories[0].Exists {
		t.Fatalf("plan.Directories = %+v, want [output Exists=false]", plan.Directories)
	}
	if !plan.NeedsAction() {
		t.Error("NeedsAction() = false, want true - notes/log.md and output/ are both missing")
	}
}

func TestPlanSetupNeedsActionFalseWhenEverythingExists(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tasks.csv"), "title\n")
	if err := os.MkdirAll(filepath.Join(root, "output"), 0o755); err != nil {
		t.Fatal(err)
	}

	setup := Setup{
		Files:       []SetupFile{{Path: "tasks.csv", Template: "title\n"}},
		Directories: []SetupDirectory{{Path: "output"}},
	}
	plan, err := PlanSetup(root, setup)
	if err != nil {
		t.Fatalf("PlanSetup() error = %v", err)
	}
	if plan.NeedsAction() {
		t.Error("NeedsAction() = true, want false - both already exist")
	}
}

func TestPlanSetupRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	for _, setup := range []Setup{
		{Files: []SetupFile{{Path: "../escape.csv"}}},
		{Files: []SetupFile{{Path: "/etc/passwd"}}},
		{Directories: []SetupDirectory{{Path: "../escape-dir"}}},
	} {
		if _, err := PlanSetup(root, setup); err == nil {
			t.Errorf("PlanSetup(%+v) error = nil, want an error for an unsafe path", setup)
		}
	}
}

func TestApplySetupCreatesOnlyWhatsMissing(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tasks.csv"), "custom content the receiver already had\n")

	setup := Setup{
		Files: []SetupFile{
			{Path: "tasks.csv", Template: "title,owner,status\n"},
			{Path: "notes/log.md", Template: "# Log\n"},
		},
		Directories: []SetupDirectory{{Path: "output"}},
	}
	plan, err := PlanSetup(root, setup)
	if err != nil {
		t.Fatalf("PlanSetup() error = %v", err)
	}
	if err := ApplySetup(root, plan); err != nil {
		t.Fatalf("ApplySetup() error = %v", err)
	}

	// The file that already existed must be left completely untouched -
	// ApplySetup never overwrites existing content.
	got, err := os.ReadFile(filepath.Join(root, "tasks.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "custom content the receiver already had\n" {
		t.Errorf("tasks.csv content = %q, want the pre-existing content preserved", got)
	}

	// The missing file and directory must now exist with the declared content.
	got, err = os.ReadFile(filepath.Join(root, "notes", "log.md"))
	if err != nil {
		t.Fatalf("expected notes/log.md to be created: %v", err)
	}
	if string(got) != "# Log\n" {
		t.Errorf("notes/log.md content = %q, want %q", got, "# Log\n")
	}
	if info, err := os.Stat(filepath.Join(root, "output")); err != nil || !info.IsDir() {
		t.Errorf("expected output/ to be created as a directory")
	}
}

// TestApplySetupDoesNotOverwriteFileCreatedAfterPlan covers a regression
// where ApplySetup trusted PlanSetup's Exists snapshot instead of
// re-checking at apply time. PlanSetup and ApplySetup are deliberately
// split around a confirmation prompt (#72), so a file can legitimately be
// created in that window - by the receiver, or by a second package. The
// stale Exists=false from the plan must not cause ApplySetup to overwrite
// whatever showed up there in the meantime.
func TestApplySetupDoesNotOverwriteFileCreatedAfterPlan(t *testing.T) {
	root := t.TempDir()
	setup := Setup{
		Files: []SetupFile{{Path: "notes/log.md", Template: "# Log\n"}},
	}
	plan, err := PlanSetup(root, setup)
	if err != nil {
		t.Fatalf("PlanSetup() error = %v", err)
	}
	if plan.Files[0].Exists {
		t.Fatalf("plan.Files[0].Exists = %v, want false (file doesn't exist yet)", plan.Files[0].Exists)
	}

	// Simulate the race: the file gets created in the window between plan
	// and apply.
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "notes", "log.md"), "written in the window between plan and apply\n")

	if err := ApplySetup(root, plan); err != nil {
		t.Fatalf("ApplySetup() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "notes", "log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "written in the window between plan and apply\n" {
		t.Errorf("notes/log.md content = %q, want the racing write preserved, not overwritten by the template", got)
	}
}

func TestApplySetupIsIdempotent(t *testing.T) {
	root := t.TempDir()
	setup := Setup{
		Files:       []SetupFile{{Path: "tasks.csv", Template: "title,owner,status\n"}},
		Directories: []SetupDirectory{{Path: "output"}},
	}

	for i := 0; i < 2; i++ {
		plan, err := PlanSetup(root, setup)
		if err != nil {
			t.Fatalf("PlanSetup() call %d error = %v", i, err)
		}
		if err := ApplySetup(root, plan); err != nil {
			t.Fatalf("ApplySetup() call %d error = %v", i, err)
		}
	}

	got, err := os.ReadFile(filepath.Join(root, "tasks.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "title,owner,status\n" {
		t.Errorf("tasks.csv content = %q after two applies, want unchanged template content", got)
	}
}
