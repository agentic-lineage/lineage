package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lineage-dev/lineage/internal/config"
	"github.com/lineage-dev/lineage/internal/packages"
)

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
	if !strings.Contains(stderr.String(), "claude") || !strings.Contains(stderr.String(), "codex") {
		t.Fatalf("stderr = %q, want it to list known providers", stderr.String())
	}
}

func TestUsageListsKnownProvidersNotHardcoded(t *testing.T) {
	var stdout bytes.Buffer
	printUsage(&stdout)
	out := stdout.String()
	if !strings.Contains(out, "claude") || !strings.Contains(out, "codex") {
		t.Fatalf("usage = %q, want it to mention every registered provider", out)
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
	cfg.Providers = map[string]config.Provider{"claude": {Binary: "/bin/echo"}}
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
