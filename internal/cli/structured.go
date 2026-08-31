package cli

import (
	"fmt"
	"io"

	"github.com/agentic-lineage/lineage/internal/packages"
	"gopkg.in/yaml.v3"
)

// PackageReport is the stable, provider-independent structured
// representation shared by `lineage package validate --yaml` and `lineage
// inspect --yaml` (#86): one schema for both, so an agent or integration
// consuming Lineage's output doesn't need to learn two different shapes
// for what is, in both cases, "here is a package." Skills/Workflows/
// Agents/Policies and Result are omitted where a command genuinely didn't
// compute them (validate doesn't run full discovery on a package that
// fails to discover cleanly; inspect has no pass/fail result) rather than
// given a placeholder value that would look like real data.
type PackageReport struct {
	Name           string                      `yaml:"name"`
	Version        string                      `yaml:"version"`
	Schema         int                         `yaml:"schema"`
	Path           string                      `yaml:"path,omitempty"`
	Digest         string                      `yaml:"digest,omitempty"`
	Description    string                      `yaml:"description,omitempty"`
	Skills         []string                    `yaml:"skills,omitempty"`
	Workflows      []string                    `yaml:"workflows,omitempty"`
	Agents         []string                    `yaml:"agents,omitempty"`
	Policies       []string                    `yaml:"policies,omitempty"`
	RequiredSkills []string                    `yaml:"required_skills"`
	Providers      []string                    `yaml:"providers"`
	Capabilities   PackageReportCapabilities   `yaml:"capabilities"`
	Portability    *packages.PortabilityReport `yaml:"portability,omitempty"`
	Notes          []string                    `yaml:"notes,omitempty"`
	Errors         []string                    `yaml:"errors,omitempty"`
	// InstructionFindings are ScanForInstructionRisk's results, computed
	// for both validate and inspect (unlike Result, which stays
	// validate-only - a finding is worth showing to a receiver deciding
	// whether to enable a package even though inspect has no pass/fail
	// concept of its own).
	InstructionFindings []InstructionRiskReport `yaml:"instruction_findings,omitempty"`
	// Result is only meaningful for validate ("pass"/"fail"); inspect
	// performs no validation, so it leaves this empty and omitted.
	Result string `yaml:"result,omitempty"`
}

type PackageReportCapabilities struct {
	FilesystemRead []string `yaml:"filesystem_read"`
	Network        []string `yaml:"network"`
}

// InstructionRiskReport is one packages.InstructionFinding rendered for
// structured output: plain strings instead of the packages package's typed
// RiskCategory/Severity, so the YAML schema doesn't leak internal Go types.
type InstructionRiskReport struct {
	Path     string `yaml:"path"`
	Category string `yaml:"category"`
	Severity string `yaml:"severity"`
	Reason   string `yaml:"reason"`
	Excerpt  string `yaml:"excerpt,omitempty"`
}

func toInstructionRiskReports(findings []packages.InstructionFinding) []InstructionRiskReport {
	if len(findings) == 0 {
		return nil
	}
	out := make([]InstructionRiskReport, len(findings))
	for i, f := range findings {
		out[i] = InstructionRiskReport{
			Path:     f.Path,
			Category: string(f.Category),
			Severity: string(f.Severity),
			Reason:   f.Reason,
			Excerpt:  f.Excerpt,
		}
	}
	return out
}

// nonNil turns a nil slice into an empty one so the YAML encoder emits `[]`
// instead of `null` for a field this command genuinely always computes - a
// consumer parsing "required_skills" or "providers" shouldn't have to
// treat "none declared" and "the field is missing" differently. Fields a
// command sometimes can't compute at all (see PackageReport's doc comment)
// stay nil instead, so they're omitted rather than rendered as a
// misleading empty list.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func inspectReport(pkg packages.Package, findings []packages.InstructionFinding) PackageReport {
	return PackageReport{
		Name:           pkg.Manifest.Name,
		Version:        pkg.Manifest.Version,
		Schema:         pkg.Manifest.Schema,
		Path:           pkg.Path,
		Digest:         pkg.Digest,
		Description:    pkg.Manifest.Description,
		Skills:         nonNil(pkg.Skills),
		Workflows:      nonNil(pkg.Workflows),
		Agents:         nonNil(pkg.Agents),
		Policies:       nonNil(pkg.Policies),
		RequiredSkills: nonNil(pkg.Manifest.Requires.Skills),
		Providers:      nonNil(pkg.Manifest.Entrypoints.Providers()),
		Capabilities: PackageReportCapabilities{
			FilesystemRead: nonNil(pkg.Manifest.Capabilities.Filesystem.Read),
			Network:        nonNil(pkg.Manifest.Capabilities.Network),
		},
		InstructionFindings: toInstructionRiskReports(findings),
	}
}

// validateReport builds a PackageReport from a validation pass. discovered
// is the result of also running Discover against the same directory,
// best-effort: a package that fails validation in a way that also breaks
// discovery (a traversing entrypoint, a declared-but-missing export) won't
// have one, and the report simply omits skills/workflows/agents/policies
// rather than guessing - report.Errors already explains why validation
// failed, discovered or not.
func validateReport(report packages.ValidateReport, discovered *packages.Package) PackageReport {
	result := "pass"
	if !report.Passed() {
		result = "fail"
	}
	pr := PackageReport{
		Name:           report.Manifest.Name,
		Version:        report.Manifest.Version,
		Schema:         report.Manifest.Schema,
		Digest:         report.Digest,
		RequiredSkills: nonNil(report.Manifest.Requires.Skills),
		Providers:      nonNil(report.Manifest.Entrypoints.Providers()),
		Capabilities: PackageReportCapabilities{
			FilesystemRead: nonNil(report.Manifest.Capabilities.Filesystem.Read),
			Network:        nonNil(report.Manifest.Capabilities.Network),
		},
		Notes:               nonNil(report.Notes),
		Errors:              nonNil(report.Errors),
		InstructionFindings: toInstructionRiskReports(report.InstructionFindings),
		Result:              result,
	}
	portability := packages.NewPortabilityReport(report)
	pr.Portability = &portability
	if discovered != nil {
		pr.Path = discovered.Path
		pr.Skills = nonNil(discovered.Skills)
		pr.Workflows = nonNil(discovered.Workflows)
		pr.Agents = nonNil(discovered.Agents)
		pr.Policies = nonNil(discovered.Policies)
	}
	return pr
}

// writeYAML encodes report and writes it to stdout. yaml.v3's Encoder is
// used directly (rather than yaml.Marshal) purely for its indent control,
// matching the two-space indent lineage.yaml manifests already use
// elsewhere in this project.
func writeYAML(stdout io.Writer, report PackageReport) error {
	enc := yaml.NewEncoder(stdout)
	enc.SetIndent(2)
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	return enc.Close()
}
