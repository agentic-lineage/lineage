package packages

import (
	"fmt"
	"io"
)

type PortabilityReport struct {
	Status                     string   `yaml:"status" json:"status"`
	ValidationStatus           string   `yaml:"validation_status" json:"validation_status"`
	Redactions                 []string `yaml:"redactions" json:"redactions"`
	ParameterizedValues        []string `yaml:"parameterized_values" json:"parameterized_values"`
	SetupRequirements          []string `yaml:"setup_requirements" json:"setup_requirements"`
	ProviderCompatibilityNotes []string `yaml:"provider_compatibility_notes" json:"provider_compatibility_notes"`
	Warnings                   []string `yaml:"warnings" json:"warnings"`
	Blockers                   []string `yaml:"blockers" json:"blockers"`
}

func (r PortabilityReport) HasBlockers() bool {
	return len(r.Blockers) > 0
}

func NewPortabilityReport(report ValidateReport) PortabilityReport {
	portability := PortabilityReport{
		Status:                     "PASS",
		ValidationStatus:           "PASS",
		Redactions:                 []string{},
		ParameterizedValues:        []string{},
		SetupRequirements:          []string{},
		ProviderCompatibilityNotes: []string{},
		Warnings:                   append([]string{}, report.Notes...),
		Blockers:                   append([]string{}, report.Errors...),
	}
	if len(portability.Warnings) > 0 {
		portability.Status = "WARN"
	}
	if len(portability.Blockers) > 0 {
		portability.Status = "BLOCKED"
		portability.ValidationStatus = "FAIL"
	}

	for _, f := range report.Manifest.Setup.Files {
		portability.SetupRequirements = append(portability.SetupRequirements, describeSetupRequirement("file", f.Path, f.Description))
	}
	for _, d := range report.Manifest.Setup.Directories {
		portability.SetupRequirements = append(portability.SetupRequirements, describeSetupRequirement("directory", d.Path, d.Description))
	}
	if report.Manifest.Entrypoints.Claude != "" {
		portability.ProviderCompatibilityNotes = append(portability.ProviderCompatibilityNotes, "claude entrypoint: "+report.Manifest.Entrypoints.Claude)
	}
	if report.Manifest.Entrypoints.Codex != "" {
		portability.ProviderCompatibilityNotes = append(portability.ProviderCompatibilityNotes, "codex entrypoint: "+report.Manifest.Entrypoints.Codex)
	}
	return portability
}

func describeSetupRequirement(kind, path, description string) string {
	if description == "" {
		return fmt.Sprintf("create %s %s", kind, path)
	}
	return fmt.Sprintf("create %s %s - %s", kind, path, description)
}

func WritePortabilityReport(w io.Writer, report PortabilityReport) {
	fmt.Fprintln(w, "portability:")
	fmt.Fprintf(w, "  status: %s\n", report.Status)
	fmt.Fprintf(w, "  validation: %s\n", report.ValidationStatus)
	writeReportList(w, "  redactions", report.Redactions)
	writeReportList(w, "  parameterized values", report.ParameterizedValues)
	writeReportList(w, "  setup requirements", report.SetupRequirements)
	writeReportList(w, "  provider compatibility", report.ProviderCompatibilityNotes)
	writeReportList(w, "  warnings", report.Warnings)
	writeReportList(w, "  blockers", report.Blockers)
}

func writeReportList(w io.Writer, label string, values []string) {
	if len(values) == 0 {
		fmt.Fprintf(w, "%s: none\n", label)
		return
	}
	fmt.Fprintf(w, "%s:\n", label)
	for _, value := range values {
		fmt.Fprintf(w, "    - %s\n", value)
	}
}
