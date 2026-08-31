package packages

import (
	"fmt"
	"path/filepath"
)

// ValidateReport is the result of Validate: every problem found, plus the
// informational facts (digest, declared capabilities) a receiver would want
// before enabling a package. Unlike Discover, which fails fast on the first
// problem so callers that actually resolve/materialize a package never
// proceed with a broken one, Validate collects every problem it finds so a
// package author sees the whole picture in one run.
type ValidateReport struct {
	Manifest Manifest
	Digest   string
	// Errors block Passed(): a declared-but-missing export, a traversing
	// entrypoint, or a secret-scan finding.
	Errors []string
	// Notes are informational only, e.g. a required skill this package
	// doesn't itself provide (it may be provided by another package at
	// enable time - Validate only sees this one package in isolation).
	Notes []string
	// InstructionFindings are the results of ScanForInstructionRisk: risky
	// agent-instruction patterns found in this package's skills, workflows,
	// agents, policies, adapters, and setup.files[].template. A
	// SeverityBlock finding here blocks Passed() exactly like an Errors
	// entry does; a SeverityWarn finding does not block on its own but
	// should still be surfaced to a receiver before enabling.
	InstructionFindings []InstructionFinding
}

// BlockingCount is the total number of problems that make Passed() false:
// every entry in Errors, plus every instruction finding severe enough to
// block on its own. Callers reporting "failed with N error(s)" should use
// this instead of len(Errors) directly, so a failure caused solely by a
// hard-stop instruction finding is still reported accurately instead of
// as "0 error(s)".
func (r ValidateReport) BlockingCount() int {
	return len(r.Errors) + len(BlockingFindings(r.InstructionFindings))
}

// Passed reports whether the package validated cleanly.
func (r ValidateReport) Passed() bool {
	return r.BlockingCount() == 0
}

// Validate runs every Phase 2 format and safety check against a package
// directory without enabling it or writing anything to disk: manifest
// schema, export authority, entrypoint path safety, a secret scan, and the
// content digest. It fails outright only if the manifest itself can't be
// loaded (schema mismatch, missing name) or a check can't run at all (I/O
// error) - anything else becomes an Errors entry so the report is complete.
func Validate(dir string) (ValidateReport, error) {
	manifest, err := LoadManifest(dir)
	if err != nil {
		return ValidateReport{}, err
	}
	report := ValidateReport{Manifest: manifest}

	for _, missing := range missingDeclared(manifest.Exports.Agents, discoverEntryNames(filepath.Join(dir, "agents"))) {
		report.Errors = append(report.Errors, fmt.Sprintf("manifest declares agent %q but it was not found", missing))
	}
	for _, missing := range missingDeclared(manifest.Exports.Workflows, discoverNamesWithFile(filepath.Join(dir, "workflows"), WorkflowFileName)) {
		report.Errors = append(report.Errors, fmt.Sprintf("manifest declares workflow %q but it was not found", missing))
	}

	if err := validateEntrypoint(dir, "claude", manifest.Entrypoints.Claude); err != nil {
		report.Errors = append(report.Errors, err.Error())
	}
	if err := validateEntrypoint(dir, "codex", manifest.Entrypoints.Codex); err != nil {
		report.Errors = append(report.Errors, err.Error())
	}

	// Setup paths are receiver-project-relative at apply time, but their
	// safety (not absolute, can't escape via "..") doesn't depend on which
	// root they're eventually resolved against - catch a bad declaration
	// here, at publish time, rather than only when some future receiver
	// tries to enable the package. dir stands in for "some root" purely to
	// exercise the same check validateEntrypoint already relies on.
	for _, f := range manifest.Setup.Files {
		if _, err := SafeJoin(dir, f.Path); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("setup.files: %v", err))
		}
	}
	for _, d := range manifest.Setup.Directories {
		if _, err := SafeJoin(dir, d.Path); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("setup.directories: %v", err))
		}
	}

	discoveredSkills := discoverSkillNames(filepath.Join(dir, "skills"))
	skillSet := make(map[string]bool, len(discoveredSkills))
	for _, s := range discoveredSkills {
		skillSet[s] = true
	}
	for _, required := range manifest.Requires.Skills {
		if !skillSet[required] {
			report.Notes = append(report.Notes, fmt.Sprintf("requires skill %q, not provided by this package alone — must come from another enabled package", required))
		}
	}

	for _, wfName := range discoverNamesWithFile(filepath.Join(dir, "workflows"), WorkflowFileName) {
		wf, err := LoadWorkflow(dir, wfName)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("workflow %q: %v", wfName, err))
			continue
		}
		for _, missing := range missingDeclared(wf.Steps, discoveredSkills) {
			report.Errors = append(report.Errors, fmt.Sprintf("workflow %q references skill %q, which was not found in this package", wfName, missing))
		}
	}

	findings, err := ScanForSecrets(dir)
	if err != nil {
		return report, err
	}
	for _, f := range findings {
		report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", f.Path, f.Reason))
	}

	instructionFindings, err := ScanForInstructionRisk(dir, manifest.Setup)
	if err != nil {
		return report, err
	}
	report.InstructionFindings = instructionFindings

	digest, err := ComputeDigest(dir)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else {
		report.Digest = digest
	}

	return report, nil
}

// missingDeclared returns every name in declared that isn't in discovered,
// in declared order. An empty declared list means "nothing declared" (not
// "declares nothing") and always returns no missing names, matching
// applyExportAuthority's fallback-to-discovery behavior.
func missingDeclared(declared, discovered []string) []string {
	if len(declared) == 0 {
		return nil
	}
	discoveredSet := make(map[string]bool, len(discovered))
	for _, name := range discovered {
		discoveredSet[name] = true
	}
	var missing []string
	for _, name := range declared {
		if !discoveredSet[name] {
			missing = append(missing, name)
		}
	}
	return missing
}
