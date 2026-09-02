package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

// noopProviderBinary returns the path to an OS-appropriate fake provider
// executable that exits successfully doing nothing. Tests that only build
// a --dry-run plan can use any string as Binary since it's never actually
// run, but tests that let `lineage run`/`workflow run` proceed to
// provider.Launch need a binary that genuinely executes on the current
// OS - "/bin/echo" doesn't exist on Windows.
func noopProviderBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "fake-provider.cmd")
		if err := os.WriteFile(path, []byte("@echo off\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, "fake-provider.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnableAndDryRun(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "agent-pack")
	if err := packages.InitPackage(pkgDir, "agent-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = map[string]config.Provider{"codex": {Binary: "/bin/echo"}}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Execute(nil, []string{"run", "codex", "--dry-run"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("dry-run error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "provider: codex") {
		t.Fatalf("dry-run output = %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "agent-pack@0.1.0") {
		t.Fatalf("dry-run output = %s", stdout.String())
	}
}

func TestEnableRejectsUnsatisfiedRequiredSkill(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "needs-review-basics")
	if err := packages.InitPackage(pkgDir, "needs-review-basics"); err != nil {
		t.Fatal(err)
	}
	manifest, err := packages.LoadManifest(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Requires.Skills = []string{"review-basics"}
	if err := packages.SaveManifest(pkgDir, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Execute(nil, []string{"enable", "./needs-review-basics"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("enable error = nil, want error for unsatisfied requires.skills")
	}
	if !strings.Contains(stderr.String(), "review-basics") {
		t.Fatalf("stderr = %q, want it to name the missing skill", stderr.String())
	}

	if _, statErr := os.Stat(config.ProjectConfigPath(project)); !os.IsNotExist(statErr) {
		t.Fatal("expected no config written when enable fails validation")
	}
}

func TestPackageValidatePasses(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "clean-pack")
	if err := packages.InitPackage(pkgDir, "clean-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "review", "SKILL.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "validate", pkgDir}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("validate error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "result: PASS") {
		t.Fatalf("stdout = %q, want PASS", stdout.String())
	}
	if !strings.Contains(stdout.String(), "digest: sha256:") {
		t.Fatalf("stdout = %q, want a printed digest", stdout.String())
	}
}

func TestPackageValidateFailsAndReportsErrors(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "broken-pack")
	if err := packages.InitPackage(pkgDir, "broken-pack"); err != nil {
		t.Fatal(err)
	}
	manifest, err := packages.LoadManifest(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Exports.Agents = []string{"missing.md"}
	if err := packages.SaveManifest(pkgDir, manifest); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Execute(nil, []string{"package", "validate", pkgDir}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("validate error = nil, want error for a failing package")
	}
	if !strings.Contains(stdout.String(), "result: FAIL") {
		t.Fatalf("stdout = %q, want FAIL", stdout.String())
	}
	if !strings.Contains(stdout.String(), "missing.md") {
		t.Fatalf("stdout = %q, want it to name the missing export", stdout.String())
	}
}

func TestPackageExportPrintsPortabilityReport(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "export-report")
	outPath := filepath.Join(root, "export-report.tgz")
	if err := packages.InitPackage(pkgDir, "export-report"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "export", pkgDir, "-o", outPath}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("export error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "portability:\n  status: PASS") {
		t.Fatalf("stdout = %q, want portability PASS report", stdout.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected export archive: %v", err)
	}
}

func TestPackageExportRefusesPortabilityBlockersBeforeWritingArchive(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "blocked-export-report")
	outPath := filepath.Join(root, "blocked-export-report.tgz")
	if err := packages.InitPackage(pkgDir, "blocked-export-report"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, ".env"), []byte("API_KEY=whatever"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"package", "export", pkgDir, "-o", outPath}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("export error = nil, want portability blocker error")
	}
	if !strings.Contains(stdout.String(), "portability:\n  status: BLOCKED") {
		t.Fatalf("stdout = %q, want portability BLOCKED report", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unresolved portability blockers") {
		t.Fatalf("stderr = %q, want portability blocker message", stderr.String())
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("archive stat error = %v, want no archive written", statErr)
	}
}

func TestRunUnknownProviderListsKnownProviders(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Execute(nil, []string{"run", "does-not-exist"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("Execute(run does-not-exist) error = nil, want error")
	}
	for _, name := range []string{"claude", "codex", "windsurf", "aider", "cline"} {
		if !strings.Contains(stderr.String(), name) {
			t.Fatalf("stderr = %q, want it to list known provider %q", stderr.String(), name)
		}
	}
}

func TestUsageListsKnownProvidersNotHardcoded(t *testing.T) {
	var stdout bytes.Buffer
	printUsage(&stdout)
	out := stdout.String()
	for _, name := range []string{"claude", "codex", "windsurf", "aider", "cline"} {
		if !strings.Contains(out, name) {
			t.Fatalf("usage = %q, want it to mention registered provider %q", out, name)
		}
	}
	if !strings.Contains(out, "put lineage in front of launchable providers on PATH") {
		t.Fatalf("usage = %q, want install-shims help to describe launchable providers", out)
	}
}

func setUpEnabledProject(t *testing.T) (project, home string) {
	t.Helper()
	tmp := t.TempDir()
	home = filepath.Join(tmp, "home")
	project = filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "agent-pack")
	if err := packages.InitPackage(pkgDir, "agent-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "hello", "SKILL.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	t.Cleanup(func() { os.Setenv(config.HomeEnv, oldHome) })
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = map[string]config.Provider{"claude": {Binary: noopProviderBinary(t)}}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	return project, home
}

func TestRunDeclinedApprovalSkipsMaterialization(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("n\n")
	if err := Execute(nil, []string{"run", "claude"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Proceed?") {
		t.Fatalf("stdout = %q, want an approval prompt", stdout.String())
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Fatalf("stdout = %q, want an abort notice", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); !os.IsNotExist(err) {
		t.Fatal("expected no materialization when approval is declined")
	}
}

func TestRunApprovedMaterializes(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\n")
	if err := Execute(nil, []string{"run", "claude"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "agent-pack-hello", "SKILL.md")); err != nil {
		t.Fatalf("expected materialized skill, stat err = %v", err)
	}
}

func TestRunClineMaterializesWithoutBinaryOrLaunch(t *testing.T) {
	project, _ := setUpEnabledProject(t)
	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = map[string]config.Provider{}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"run", "cline", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Cline run error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".clinerules", "lineage.md")); err != nil {
		t.Fatalf("expected Cline context materialization: %v", err)
	}
	if strings.Contains(stderr.String(), "binary") || strings.Contains(stdout.String(), "launch failed") {
		t.Fatalf("Cline run attempted launch: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunYesFlagSkipsPrompt(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"run", "claude", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "Proceed?") {
		t.Fatalf("stdout = %q, want no prompt with --yes", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "agent-pack-hello")); err != nil {
		t.Fatalf("expected materialized skill with --yes, stat err = %v", err)
	}
}

func TestRunDoesNotRePromptWhenUnchanged(t *testing.T) {
	_, _ = setUpEnabledProject(t)

	var stdout1, stderr1 bytes.Buffer
	if err := Execute(nil, []string{"run", "claude", "--yes"}, nil, &stdout1, &stderr1); err != nil {
		t.Fatalf("first run error = %v stderr=%s", err, stderr1.String())
	}

	// No stdin at all this time - if a second, identical run tried to
	// prompt, confirm() would read from a nil reader and decline, and the
	// materialized skill would get removed. It must not, since nothing
	// about the enabled package set changed.
	var stdout2, stderr2 bytes.Buffer
	if err := Execute(nil, []string{"run", "claude"}, nil, &stdout2, &stderr2); err != nil {
		t.Fatalf("second run error = %v stderr=%s", err, stderr2.String())
	}
	if strings.Contains(stdout2.String(), "Proceed?") {
		t.Fatalf("second run stdout = %q, want no re-prompt for an unchanged package set", stdout2.String())
	}
}

func TestPackageExportWritesArchive(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "export-me")
	if err := packages.InitPackage(pkgDir, "export-me"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "review", "SKILL.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(root, "out.tgz")
	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "export", pkgDir, "-o", outPath}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("export error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected archive at %s: %v", outPath, err)
	}
}

func TestPackageExportRefusesInvalidPackage(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "broken")
	if err := packages.InitPackage(pkgDir, "broken"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(root, "out.tgz")
	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"package", "export", pkgDir, "-o", outPath}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("export error = nil, want error for a package with a secret finding")
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatal("expected no archive left behind when export fails")
	}
}

func TestPackageExportThenImportRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	pkgDir := filepath.Join(tmp, "roundtrip-pack")
	if err := packages.InitPackage(pkgDir, "roundtrip-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "review", "SKILL.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)

	archivePath := filepath.Join(tmp, "roundtrip-pack.tgz")
	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "export", pkgDir, "-o", archivePath}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("export error = %v stderr=%s", err, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := Execute(nil, []string{"package", "import", archivePath}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("import error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "imported package roundtrip-pack") {
		t.Fatalf("import stdout = %q, want it to name the imported package", stdout.String())
	}

	importedDir := filepath.Join(config.UserPackagesDir(home), "roundtrip-pack")
	if _, err := os.Stat(filepath.Join(importedDir, "skills", "review", "SKILL.md")); err != nil {
		t.Fatalf("expected imported skill file: %v", err)
	}

	original, err := packages.Discover(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := packages.Discover(importedDir)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Digest != original.Digest {
		t.Fatalf("imported digest = %q, want %q", imported.Digest, original.Digest)
	}
}

func TestListShowsEnabledPackages(t *testing.T) {
	setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"list"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("list error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "agent-pack@0.1.0") {
		t.Fatalf("list output = %q, want it to include the enabled package", stdout.String())
	}
}

func TestListWithNoEnabledPackagesSaysSo(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), config.DefaultProjectConfig()); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"list"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("list error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no packages enabled") {
		t.Fatalf("list output = %q, want a clear empty message", stdout.String())
	}
}

func TestDisableCleansUpMaterializedContent(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"run", "claude", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "agent-pack-hello")); err != nil {
		t.Fatalf("expected materialized skill before disable: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	// --yes: disabling the only enabled package changes what's
	// materialized, so it now goes through the same confirmation gate
	// enable/run/workflow run already use (#160).
	if err := Execute(nil, []string{"disable", "./agent-pack", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("disable error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "agent-pack-hello")); !os.IsNotExist(err) {
		t.Fatalf("expected staged skill removed after disable, stat err = %v", err)
	}

	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	if contains(cfg.EnabledPackages, "./agent-pack") {
		t.Fatalf("EnabledPackages = %#v, want ./agent-pack removed", cfg.EnabledPackages)
	}
}

// TestDisableRequiresConfirmationBeforeRematerializing covers a regression
// where disable was the one mutating command with no confirmation gate at
// all - every other command that writes staged files (enable, run,
// workflow run) asks first. Declining must leave both the config and the
// already-staged content completely untouched, the same as declining any
// other prompt in this codebase.
func TestDisableRequiresConfirmationBeforeRematerializing(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"run", "claude", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	stdin := strings.NewReader("n\n")
	if err := Execute(nil, []string{"disable", "./agent-pack"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("disable error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("stdout = %q, want it to explain the disable was aborted", stdout.String())
	}

	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "agent-pack-hello")); err != nil {
		t.Errorf("expected the staged skill to survive a declined disable: %v", err)
	}
	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(cfg.EnabledPackages, "./agent-pack") {
		t.Errorf("EnabledPackages = %#v, want ./agent-pack still enabled after declining", cfg.EnabledPackages)
	}
}

func TestDisableAcceptsInteractiveConfirmation(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"run", "claude", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	stdin := strings.NewReader("y\n")
	if err := Execute(nil, []string{"disable", "./agent-pack"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("disable error = %v stderr=%s", err, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "agent-pack-hello")); !os.IsNotExist(err) {
		t.Errorf("expected staged skill removed after an approved disable, stat err = %v", err)
	}
}

func TestDisableUnknownRefFails(t *testing.T) {
	setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"disable", "./not-enabled"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("disable error = nil, want error for a ref that isn't enabled")
	}
}

func TestInspectShowsPackageWithoutEnabling(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	pkgDir := filepath.Join(tmp, "standalone-pack")
	if err := packages.InitPackage(pkgDir, "standalone-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "review", "SKILL.md"), []byte("# Review"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.HomeEnv, home)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"inspect", pkgDir}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("inspect error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "standalone-pack@0.1.0") {
		t.Fatalf("inspect output = %q, want package identity", stdout.String())
	}
	if !strings.Contains(stdout.String(), "skills: review") {
		t.Fatalf("inspect output = %q, want discovered skills", stdout.String())
	}

	cfgPath := config.ProjectConfigPath(filepath.Dir(pkgDir))
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatal("inspect must not create a project config or enable anything")
	}
}

func TestDoctorOutsideProjectSkipsPackageChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"doctor"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("doctor error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not inside a Lineage project") {
		t.Fatalf("doctor output = %q, want it to note it's outside a project", stdout.String())
	}
	if !strings.Contains(stdout.String(), "result: OK") {
		t.Fatalf("doctor output = %q, want a clean result outside a project", stdout.String())
	}
}

func TestDoctorFailsOnUnresolvableEnabledPackage(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultProjectConfig()
	cfg.EnabledPackages = []string{"./does-not-exist"}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Execute(nil, []string{"doctor"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("doctor error = nil, want error for an unresolvable enabled package")
	}
	if !strings.Contains(stdout.String(), "enabled packages: FAIL") {
		t.Fatalf("doctor output = %q, want a FAIL line for enabled packages", stdout.String())
	}
}

func TestDoctorReportsEachKnownProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"doctor"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("doctor error = %v stderr=%s", err, stderr.String())
	}
	for _, name := range []string{"claude", "codex"} {
		if !strings.Contains(stdout.String(), "provider "+name+":") {
			t.Fatalf("doctor output = %q, want a line for provider %s", stdout.String(), name)
		}
	}
}

func setUpEnabledWorkflowProject(t *testing.T) (project, home string) {
	t.Helper()
	tmp := t.TempDir()
	home = filepath.Join(tmp, "home")
	project = filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "wf-pack")
	if err := packages.InitPackage(pkgDir, "wf-pack"); err != nil {
		t.Fatal(err)
	}
	for _, skill := range []string{"gather", "review"} {
		if err := os.MkdirAll(filepath.Join(pkgDir, "skills", skill), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "skills", skill, "SKILL.md"), []byte("# "+skill), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "workflows", "review-flow"), 0o755); err != nil {
		t.Fatal(err)
	}
	wfContent := "---\nsteps:\n  - gather\n  - review\n---\n\n# Review Flow\n"
	if err := os.WriteFile(filepath.Join(pkgDir, "workflows", "review-flow", "WORKFLOW.md"), []byte(wfContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	t.Cleanup(func() { os.Setenv(config.HomeEnv, oldHome) })
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./wf-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = map[string]config.Provider{"claude": {Binary: noopProviderBinary(t)}}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}
	return project, home
}

func TestWorkflowRunDryRunShowsOrderedSteps(t *testing.T) {
	setUpEnabledWorkflowProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"workflow", "run", "review-flow", "claude", "--dry-run"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("workflow run error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1. gather") || !strings.Contains(stdout.String(), "2. review") {
		t.Fatalf("dry-run output = %s, want ordered steps", stdout.String())
	}
}

func TestWorkflowRunMaterializesOnlyItsSteps(t *testing.T) {
	project, _ := setUpEnabledWorkflowProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"workflow", "run", "review-flow", "claude", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("workflow run error = %v stderr=%s", err, stderr.String())
	}
	for _, skill := range []string{"gather", "review"} {
		if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "wf-pack-"+skill)); err != nil {
			t.Fatalf("expected %s staged: %v", skill, err)
		}
	}

	content, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Active workflow: review-flow") {
		t.Fatalf("CLAUDE.md = %s, want the active workflow sequence", content)
	}
}

func TestWorkflowRunClineMaterializesWithoutBinaryOrLaunch(t *testing.T) {
	project, _ := setUpEnabledWorkflowProject(t)
	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = map[string]config.Provider{}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"workflow", "run", "review-flow", "cline", "--dry-run"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Cline workflow dry-run error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "real_binary: none") || !strings.Contains(stdout.String(), "launch: disabled (config/materialization only)") {
		t.Fatalf("Cline workflow dry-run = %q, want materialization-only output", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := Execute(nil, []string{"workflow", "run", "review-flow", "cline", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Cline workflow run error = %v stderr=%s", err, stderr.String())
	}
	content, err := os.ReadFile(filepath.Join(project, ".clinerules", "lineage.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Active workflow: review-flow") {
		t.Fatalf("Cline context = %s, want active workflow", content)
	}
}

func TestWorkflowRunWindsurfMaterializesWithoutLaunching(t *testing.T) {
	project, _ := setUpEnabledWorkflowProject(t)
	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Providers = map[string]config.Provider{"windsurf": {Binary: filepath.Join(project, "must-not-launch")}}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"workflow", "run", "review-flow", "windsurf", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("workflow run error = %v stderr=%s", err, stderr.String())
	}
	for _, skill := range []string{"gather", "review"} {
		if _, err := os.Stat(filepath.Join(project, ".windsurf", "rules", "wf-pack-"+skill)); err != nil {
			t.Fatalf("expected %s staged: %v", skill, err)
		}
	}
}

func TestWorkflowRunUnknownWorkflowFails(t *testing.T) {
	setUpEnabledWorkflowProject(t)

	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"workflow", "run", "does-not-exist", "claude", "--dry-run"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("workflow run error = nil, want error for an unknown workflow")
	}
}
