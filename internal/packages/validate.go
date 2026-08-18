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
}

// Passed reports whether the package validated cleanly.
func (r ValidateReport) Passed() bool {
	return len(r.Errors) == 0
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
	for _, missing := range missingDeclared(manifest.Exports.Workflows, discoverNamesWithFile(filepath.Join(dir, "workflows"), "WORKFLOW.md")) {
		report.Errors = append(report.Errors, fmt.Sprintf("manifest declares workflow %q but it was not found", missing))
	}

	if err := validateEntrypoint(dir, "claude", manifest.Entrypoints.Claude); err != nil {
		report.Errors = append(report.Errors, err.Error())
	}
	if err := validateEntrypoint(dir, "codex", manifest.Entrypoints.Codex); err != nil {
		report.Errors = append(report.Errors, err.Error())
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

	findings, err := ScanForSecrets(dir)
	if err != nil {
		return report, err
	}
	for _, f := range findings {
		report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", f.Path, f.Reason))
	}

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
