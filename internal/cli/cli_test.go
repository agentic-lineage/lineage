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
	if err := Execute(nil, []string{"enable", "./agent-pack"}, &stdout, &stderr); err != nil {
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
	if err := Execute(nil, []string{"run", "codex", "--dry-run"}, &stdout, &stderr); err != nil {
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
	err = Execute(nil, []string{"enable", "./needs-review-basics"}, &stdout, &stderr)
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
	if err := Execute(nil, []string{"package", "validate", pkgDir}, &stdout, &stderr); err != nil {
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
	err = Execute(nil, []string{"package", "validate", pkgDir}, &stdout, &stderr)
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
