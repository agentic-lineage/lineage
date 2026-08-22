package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
	"gopkg.in/yaml.v3"
)

func decodeReport(t *testing.T, out string) PackageReport {
	t.Helper()
	var report PackageReport
	if err := yaml.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output is not valid YAML: %v\noutput:\n%s", err, out)
	}
	return report
}

func TestPackageValidateYAMLReportsFullPackage(t *testing.T) {
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
	manifest, err := packages.LoadManifest(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Entrypoints.Claude = "skills/review/SKILL.md"
	if err := packages.SaveManifest(pkgDir, manifest); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "validate", pkgDir, "--yaml"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("validate --yaml error = %v stderr=%s", err, stderr.String())
	}

	report := decodeReport(t, stdout.String())
	if report.Name != "clean-pack" || report.Result != "pass" {
		t.Fatalf("report = %+v, want name=clean-pack result=pass", report)
	}
	if report.Digest == "" {
		t.Error("report.Digest is empty")
	}
	if len(report.Skills) != 1 || report.Skills[0] != "review" {
		t.Errorf("report.Skills = %v, want [review]", report.Skills)
	}
	if len(report.Providers) != 1 || report.Providers[0] != "claude" {
		t.Errorf("report.Providers = %v, want [claude]", report.Providers)
	}
	if len(report.Errors) != 0 {
		t.Errorf("report.Errors = %v, want none", report.Errors)
	}
	if report.Portability == nil || report.Portability.Status != "PASS" {
		t.Errorf("report.Portability = %+v, want PASS portability report", report.Portability)
	}
}

func TestPackageValidateYAMLFailureExitsNonZeroWithErrors(t *testing.T) {
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
	err = Execute(nil, []string{"package", "validate", pkgDir, "--yaml"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("validate --yaml error = nil, want a non-nil error (non-zero exit) for a failing package")
	}

	// Even on failure, stdout must still be the structured report, not the
	// error text - a consumer parsing stdout as YAML shouldn't have to
	// branch on the exit code first to know what stdout contains.
	report := decodeReport(t, stdout.String())
	if report.Result != "fail" {
		t.Errorf("report.Result = %q, want fail", report.Result)
	}
	if len(report.Errors) == 0 {
		t.Error("report.Errors is empty, want the missing-export error listed")
	}
	if report.Portability == nil || report.Portability.Status != "BLOCKED" {
		t.Errorf("report.Portability = %+v, want BLOCKED portability report", report.Portability)
	}
}

func TestInspectYAMLReportsFullPackage(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "resume-pack")
	if err := packages.InitPackage(pkgDir, "resume-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "polish"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "polish", "SKILL.md"), []byte("# Polish"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := packages.LoadManifest(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Capabilities.Network = []string{"api.example.com"}
	if err := packages.SaveManifest(pkgDir, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"inspect", "./resume-pack", "--yaml"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("inspect --yaml error = %v stderr=%s", err, stderr.String())
	}

	report := decodeReport(t, stdout.String())
	if report.Name != "resume-pack" {
		t.Errorf("report.Name = %q, want resume-pack", report.Name)
	}
	if len(report.Skills) != 1 || report.Skills[0] != "polish" {
		t.Errorf("report.Skills = %v, want [polish]", report.Skills)
	}
	if len(report.Capabilities.Network) != 1 || report.Capabilities.Network[0] != "api.example.com" {
		t.Errorf("report.Capabilities.Network = %v, want [api.example.com]", report.Capabilities.Network)
	}
	// inspect performs no validation, so Result must be absent, not an
	// empty string masquerading as a real value.
	if report.Result != "" {
		t.Errorf("report.Result = %q, want empty (inspect has no pass/fail concept)", report.Result)
	}
}
