// Package analysis is the agent-assisted analysis stage (#104) that sits
// between internal/inventory's evidence and internal/model's behavioral
// model: it hands a Provider the source inventory as evidence, parses and
// validates whatever BehavioralModel comes back, and fails closed if that
// output is malformed, invalid, or contradicts the evidence it was given.
//
// It never edits the source workspace, executes anything in it, or writes
// package artifacts — this stage only produces a validated (or rejected)
// BehavioralModel for a human, or a later compilation stage (#106), to act
// on.
package analysis

import (
	"context"
	"fmt"
	"sort"

	"github.com/agentic-lineage/lineage/internal/inventory"
	"github.com/agentic-lineage/lineage/internal/model"
)

// Provider turns source evidence into a candidate behavioral model. It
// returns the provider's raw output, not a parsed model.BehavioralModel:
// parsing and validation happen exactly once, in Analyze, so a malformed
// response is always caught in the same place regardless of which Provider
// produced it.
type Provider interface {
	Analyze(ctx context.Context, inv inventory.Inventory) ([]byte, error)
}

// Result is one analysis pass's outcome. Model is populated whenever
// parsing succeeds, even if Report fails — a caller can see exactly what
// the provider produced and why it was rejected. Callers must check
// Report.Passed() before trusting Model; Model alone is never a signal
// that analysis succeeded.
type Result struct {
	Inventory inventory.Inventory
	Model     model.BehavioralModel
	Report    model.ValidateReport
}

// Analyze runs one analysis pass: it hands inv to p as evidence, parses and
// validates whatever model p returns via model.ParseModel and
// model.Validate, and layers a small set of provider-output-quality checks
// on top — see qualityNotes. Those checks judge how carefully the analysis
// was done, not model/inventory consistency (that's model.Validate's job),
// so they land in Report.Notes, never Report.Errors: a provider that did
// something worth double-checking is not the same as one whose model is
// actually invalid.
//
// inv is discovered once by the caller and threaded through unchanged to
// both the provider call and the final Validate — Analyze never
// re-discovers it, so every check in one pass agrees on exactly which
// source snapshot it's judging evidence against.
func Analyze(ctx context.Context, p Provider, inv inventory.Inventory) (Result, error) {
	raw, err := p.Analyze(ctx, inv)
	if err != nil {
		return Result{}, fmt.Errorf("provider analysis failed: %w", err)
	}

	m, err := model.ParseModel(raw)
	if err != nil {
		return Result{}, fmt.Errorf("parse provider output: %w", err)
	}

	report, err := model.Validate(m, inv)
	if err != nil {
		return Result{}, fmt.Errorf("validate provider output: %w", err)
	}
	report.Notes = append(report.Notes, qualityNotes(m, inv)...)

	return Result{Inventory: inv, Model: m, Report: report}, nil
}

// qualityNotes flags provider output that is technically valid (it passes
// model.Validate) but not trustworthy enough to accept without a second
// look:
//
//   - evidence with no supporting quote (EvidenceRef.Note), so a reviewer
//     has nothing to spot-check the claim against without reopening the
//     source file;
//   - evidence citing a file whose basename is ambiguous
//     (inventory.Entry.AmbiguousBasename) with no Line or Note pinning
//     which occurrence is meant — path+digest still resolve, so
//     model.Validate accepts it, but nothing disambiguates which mention
//     the claim is actually about;
//   - a suspiciously high decision-to-step ratio, which can mean the
//     provider punted on ambiguity it could have resolved from the
//     evidence it was given rather than doing the analysis.
//
// These are judgment calls about analysis quality, not model/inventory
// consistency, so they don't belong in model.Validate itself — they live
// here, specific to this stage.
func qualityNotes(m model.BehavioralModel, inv inventory.Inventory) []string {
	entries := make(map[string]inventory.Entry, len(inv.Entries))
	for _, e := range inv.Entries {
		entries[e.Path] = e
	}

	var notes []string
	checkRefs := func(context string, refs []model.EvidenceRef) {
		for _, ref := range refs {
			entry, known := entries[ref.Path]
			switch {
			case known && entry.AmbiguousBasename && ref.Line == 0 && ref.Note == "":
				notes = append(notes, fmt.Sprintf("%s: evidence cites %q, whose basename is ambiguous, with no line or note to disambiguate which file is meant", context, ref.Path))
			case ref.Note == "":
				notes = append(notes, fmt.Sprintf("%s: evidence for %q has no supporting note/quote", context, ref.Path))
			}
		}
	}

	for _, step := range m.Steps {
		checkRefs(fmt.Sprintf("step %q", step.ID), step.Evidence)
		for _, claims := range [][]model.Claim{step.Inputs, step.Outputs, step.Skills, step.Tools, step.References} {
			for _, c := range claims {
				checkRefs(fmt.Sprintf("step %q claim %q", step.ID, c.Value), c.Evidence)
			}
		}
	}
	for _, d := range m.Decisions {
		checkRefs(fmt.Sprintf("decision %q", d.ID), d.Evidence)
	}

	if len(m.Steps) > 0 && len(m.Decisions) > len(m.Steps) {
		notes = append(notes, fmt.Sprintf("decision-to-step ratio is high (%d decisions for %d steps) — review whether resolvable ambiguity was deferred instead of analyzed", len(m.Decisions), len(m.Steps)))
	}

	sort.Strings(notes)
	return notes
}
