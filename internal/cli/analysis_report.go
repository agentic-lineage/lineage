package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agentic-lineage/lineage/internal/analysis"
	"github.com/agentic-lineage/lineage/internal/model"
	"gopkg.in/yaml.v3"
)

// EvidenceLocation is a click-through pointer to the source file (and,
// where known, line) backing a step or decision - what makes
// AnalysisReport navigable rather than just a summary.
type EvidenceLocation struct {
	Path string `yaml:"path"`
	Line int    `yaml:"line,omitempty"`
}

type InventorySummary struct {
	Root       string         `yaml:"root"`
	TotalFiles int            `yaml:"total_files"`
	ByKind     map[string]int `yaml:"by_kind"`
}

type StepSummary struct {
	ID          string             `yaml:"id"`
	Name        string             `yaml:"name"`
	Description string             `yaml:"description,omitempty"`
	Evidence    []EvidenceLocation `yaml:"evidence"`
}

type DecisionSummary struct {
	ID          string             `yaml:"id"`
	Description string             `yaml:"description"`
	Refs        []string           `yaml:"refs,omitempty"`
	Evidence    []EvidenceLocation `yaml:"evidence"`
}

// AnalysisReport is `lineage analyze`'s stable, provider-independent
// structured output, mirroring PackageReport's role for `package validate`/
// `inspect`. Fields are chosen so an author can navigate straight to the
// source file(s) behind any step or decision, not just read a summary -
// see EvidenceLocation on StepSummary/DecisionSummary.
type AnalysisReport struct {
	// Provider distinguishes a real analysis run from a fixture-driven test
	// run ("claude" vs "fixture:<path>") - both can produce an identically-
	// shaped clean report, so without this field a leftover --fixture flag
	// could make a canned test response look like a trusted real analysis.
	Provider              string            `yaml:"provider"`
	SourceInventoryDigest string            `yaml:"source_inventory_digest"`
	Inventory             InventorySummary  `yaml:"inventory"`
	Steps                 []StepSummary     `yaml:"inferred_behavior"`
	Decisions             []DecisionSummary `yaml:"decisions_needing_review"`
	Result                string            `yaml:"result"`
	Errors                []string          `yaml:"errors,omitempty"`
	Notes                 []string          `yaml:"notes,omitempty"`
}

// buildAnalysisReport translates an analysis.Result into the CLI-facing
// AnalysisReport shape.
func buildAnalysisReport(providerLabel string, result analysis.Result) AnalysisReport {
	byKind := make(map[string]int)
	for _, e := range result.Inventory.Entries {
		byKind[string(e.Kind)]++
	}

	steps := make([]StepSummary, 0, len(result.Model.Steps))
	for _, s := range result.Model.Steps {
		steps = append(steps, StepSummary{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Evidence:    stepEvidenceLocations(s),
		})
	}

	decisions := make([]DecisionSummary, 0, len(result.Model.Decisions))
	for _, d := range result.Model.Decisions {
		refs := make([]string, 0, len(d.Refs))
		for _, r := range d.Refs {
			refs = append(refs, refString(r))
		}
		decisions = append(decisions, DecisionSummary{
			ID:          d.ID,
			Description: d.Description,
			Refs:        refs,
			Evidence:    evidenceLocations(d.Evidence),
		})
	}

	resultStr := "pass"
	if !result.Report.Passed() {
		resultStr = "fail"
	}

	return AnalysisReport{
		Provider:              providerLabel,
		SourceInventoryDigest: result.Model.SourceInventoryDigest,
		Inventory: InventorySummary{
			Root:       result.Inventory.Root,
			TotalFiles: len(result.Inventory.Entries),
			ByKind:     byKind,
		},
		Steps:     steps,
		Decisions: decisions,
		Result:    resultStr,
		Errors:    result.Report.Errors,
		Notes:     result.Report.Notes,
	}
}

// stepEvidenceLocations collects every evidence location backing s - its
// own step-level evidence plus every claim's evidence across all fields -
// deduplicated, so an author sees every source file behind a step without
// digging into each claim individually.
func stepEvidenceLocations(s model.Step) []EvidenceLocation {
	var refs []model.EvidenceRef
	refs = append(refs, s.Evidence...)
	for _, claims := range [][]model.Claim{s.Inputs, s.Outputs, s.Skills, s.Tools, s.References} {
		for _, c := range claims {
			refs = append(refs, c.Evidence...)
		}
	}
	return evidenceLocations(refs)
}

func evidenceLocations(refs []model.EvidenceRef) []EvidenceLocation {
	seen := make(map[EvidenceLocation]bool)
	var locs []EvidenceLocation
	for _, ref := range refs {
		loc := EvidenceLocation{Path: ref.Path, Line: ref.Line}
		if seen[loc] {
			continue
		}
		seen[loc] = true
		locs = append(locs, loc)
	}
	return locs
}

// refString renders a model.Ref as a short, human-readable pointer to what
// a Decision blocks - "model-level" for an unscoped Ref, otherwise as much
// of "step <id> field <field> key <key>" as is set.
func refString(r model.Ref) string {
	if r.StepID == "" {
		return "model-level"
	}
	s := fmt.Sprintf("step %s", r.StepID)
	if r.Field != "" {
		s += fmt.Sprintf(" field %s", r.Field)
	}
	if r.Key != "" {
		s += fmt.Sprintf(" key %s", r.Key)
	}
	return s
}

func evidenceLocationString(e EvidenceLocation) string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d", e.Path, e.Line)
	}
	return e.Path
}

// printAnalysisReport writes r's human-readable form, with source
// inventory, inferred behavior, and decisions needing author review in
// clearly separated sections, per #104's acceptance criteria.
func printAnalysisReport(stdout io.Writer, r AnalysisReport) {
	fmt.Fprintf(stdout, "provider: %s\n", r.Provider)
	fmt.Fprintf(stdout, "source_inventory_digest: %s\n\n", r.SourceInventoryDigest)

	fmt.Fprintf(stdout, "inventory:\n")
	fmt.Fprintf(stdout, "  root: %s\n", r.Inventory.Root)
	fmt.Fprintf(stdout, "  total_files: %d\n", r.Inventory.TotalFiles)
	kinds := make([]string, 0, len(r.Inventory.ByKind))
	for k := range r.Inventory.ByKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(stdout, "  %s: %d\n", k, r.Inventory.ByKind[k])
	}
	fmt.Fprintln(stdout)

	fmt.Fprintf(stdout, "inferred behavior (%d step(s)):\n", len(r.Steps))
	for _, s := range r.Steps {
		fmt.Fprintf(stdout, "  - %s: %s\n", s.ID, s.Name)
		if s.Description != "" {
			fmt.Fprintf(stdout, "      %s\n", s.Description)
		}
		for _, e := range s.Evidence {
			fmt.Fprintf(stdout, "      evidence: %s\n", evidenceLocationString(e))
		}
	}
	fmt.Fprintln(stdout)

	fmt.Fprintf(stdout, "decisions needing author review (%d):\n", len(r.Decisions))
	for _, d := range r.Decisions {
		fmt.Fprintf(stdout, "  - %s: %s\n", d.ID, d.Description)
		for _, ref := range d.Refs {
			fmt.Fprintf(stdout, "      blocks: %s\n", ref)
		}
		for _, e := range d.Evidence {
			fmt.Fprintf(stdout, "      evidence: %s\n", evidenceLocationString(e))
		}
	}
	fmt.Fprintln(stdout)

	if len(r.Notes) > 0 {
		fmt.Fprintf(stdout, "notes:\n")
		for _, n := range r.Notes {
			fmt.Fprintf(stdout, "  - %s\n", n)
		}
		fmt.Fprintln(stdout)
	}

	if len(r.Errors) > 0 {
		fmt.Fprintf(stdout, "errors:\n")
		for _, e := range r.Errors {
			fmt.Fprintf(stdout, "  - %s\n", e)
		}
		fmt.Fprintln(stdout)
	}

	fmt.Fprintf(stdout, "result: %s\n", strings.ToUpper(r.Result))
}

// writeAnalysisYAML mirrors writeYAML's encoding conventions for
// AnalysisReport - kept as a separate function rather than a generic
// helper since PackageReport's own writer isn't generic either (one type,
// one function, following this package's existing pattern).
func writeAnalysisYAML(stdout io.Writer, report AnalysisReport) error {
	enc := yaml.NewEncoder(stdout)
	enc.SetIndent(2)
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	return enc.Close()
}
