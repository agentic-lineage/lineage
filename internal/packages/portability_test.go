package packages

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortabilityReportCleanPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clean-portable")
	if err := InitPackage(root, "clean-portable"); err != nil {
		t.Fatal(err)
	}

	validateReport, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	report := NewPortabilityReport(validateReport)
	if report.Status != "PASS" || report.ValidationStatus != "PASS" || report.HasBlockers() {
		t.Fatalf("portability report = %+v, want clean pass", report)
	}
}

func TestPortabilityReportWarningOnlyPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "warning-portable")
	if err := InitPackage(root, "warning-portable"); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Requires.Skills = []string{"security-basics"}
	manifest.Setup.Files = []SetupFile{{Path: "tasks.csv", Description: "tracks work"}}
	manifest.Entrypoints.Codex = "skills/security-basics/SKILL.md"
	if err := SaveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}

	validateReport, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	report := NewPortabilityReport(validateReport)
	if report.Status != "WARN" || report.ValidationStatus != "PASS" || report.HasBlockers() {
		t.Fatalf("portability report = %+v, want warning-only pass", report)
	}
	if len(report.Warnings) != 1 || len(report.SetupRequirements) != 1 || len(report.ProviderCompatibilityNotes) != 1 {
		t.Fatalf("portability report = %+v, want validation note, setup requirement, and provider note", report)
	}
}

func TestPortabilityReportBlockerPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blocked-portable")
	if err := InitPackage(root, "blocked-portable"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".env"), "API_KEY=whatever")

	validateReport, err := Validate(root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	report := NewPortabilityReport(validateReport)
	if report.Status != "BLOCKED" || report.ValidationStatus != "FAIL" || !report.HasBlockers() {
		t.Fatalf("portability report = %+v, want blocked fail", report)
	}

	var out bytes.Buffer
	WritePortabilityReport(&out, report)
	if !strings.Contains(out.String(), "blockers:") || !strings.Contains(out.String(), ".env:") {
		t.Fatalf("rendered portability report = %q, want blocker details", out.String())
	}
}
