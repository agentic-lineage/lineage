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
	err = Execute(nil, []string{"run", "does-not-exist"}, &stdout, &stderr)
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
