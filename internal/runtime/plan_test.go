package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

func TestBuildPlanDryRun(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "agent-pack")
	if err := packages.InitPackage(pkgDir, "agent-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(pkgDir, "skills", "review", "SKILL.md"), "# Review")
	mustWrite(t, filepath.Join(pkgDir, "workflows", "ship", "WORKFLOW.md"), "# Ship")

	cfg := config.ProjectConfig{
		Workspace:       "makers",
		EnabledPackages: []string{"./agent-pack"},
		Providers: map[string]config.Provider{
			"claude": {Binary: "/bin/echo", Args: []string{"base"}},
		},
	}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan("claude", project, home, []string{"task"})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.ProviderPlan.Binary != "/bin/echo" {
		t.Fatalf("Binary = %q", plan.ProviderPlan.Binary)
	}
	if got := strings.Join(plan.ProviderPlan.Args, " "); got != "base task" {
		t.Fatalf("Args = %q", got)
	}

	dryRun := plan.DryRunString()
	for _, needle := range []string{"provider: claude", "workspace: makers", "agent-pack@0.1.0", "skills: review", "workflows: ship", "digest: sha256:", "capabilities:", "filesystem.read: none", "network: none"} {
		if !strings.Contains(dryRun, needle) {
			t.Fatalf("DryRunString() missing %q:\n%s", needle, dryRun)
		}
	}
}

func TestBuildPlanUnknownProvider(t *testing.T) {
	_, err := BuildPlan("unknown", t.TempDir(), t.TempDir(), nil)
	if err == nil {
		t.Fatal("BuildPlan() error = nil, want error")
	}
}

func TestBuildPlanClineDryRun(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	cfg := config.DefaultProjectConfig()
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan("cline", project, home, []string{"must-not-launch"})
	if err != nil {
		t.Fatalf("BuildPlan(cline) error = %v", err)
	}
	if !strings.Contains(plan.DryRunString(), "provider: cline") {
		t.Fatalf("DryRunString() = %q, want Cline provider", plan.DryRunString())
	}
	if plan.ProviderPlan.Binary != "" || !plan.ProviderPlan.MaterializeOnly ||
		!strings.Contains(plan.DryRunString(), "real_binary: none") ||
		!strings.Contains(plan.DryRunString(), "args: must-not-launch") ||
		!strings.Contains(plan.DryRunString(), "launch: disabled (config/materialization only)") {
		t.Fatalf("Cline plan = %#v, dry run = %q; want no launch target", plan.ProviderPlan, plan.DryRunString())
	}
}

func TestBuildPlanAiderDryRun(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	cfg := config.ProjectConfig{Providers: map[string]config.Provider{"aider": {Binary: "/bin/echo"}}}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan("aider", project, home, nil)
	if err != nil {
		t.Fatalf("BuildPlan(aider) error = %v", err)
	}
	if !strings.Contains(plan.DryRunString(), "provider: aider") {
		t.Fatalf("DryRunString() = %q, want Aider provider", plan.DryRunString())
	}
}

func TestBuildPlanWindsurfDryRun(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	cfg := config.ProjectConfig{
		Providers: map[string]config.Provider{
			"windsurf": {Binary: "/bin/echo"},
		},
	}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan("windsurf", project, home, nil)
	if err != nil {
		t.Fatalf("BuildPlan(windsurf) error = %v", err)
	}
	if !strings.Contains(plan.DryRunString(), "provider: windsurf") {
		t.Fatalf("DryRunString() = %q, want Windsurf provider", plan.DryRunString())
	}
	if !strings.Contains(plan.DryRunString(), "real_binary: none") ||
		!strings.Contains(plan.DryRunString(), "launch: disabled (config/materialization only)") {
		t.Fatalf("DryRunString() = %q, want materialization-only plan", plan.DryRunString())
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
