package model

import (
	"fmt"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

// ValidateReport is the result of Validate: every problem found, plus
// informational facts. Follows packages.ValidateReport's shape and
// rationale exactly: Validate collects every problem it finds in one run
// (unlike a fail-fast loader) so whoever built the model sees the whole
// picture at once.
type ValidateReport struct {
	Errors []string
	Notes  []string
}

// Passed reports whether the model validated cleanly.
func (r ValidateReport) Passed() bool {
	return len(r.Errors) == 0
}

// Validate checks m for internal consistency and for evidence that still
// resolves against inv, the inventory m claims to have been built from. It
// never fails fast except when m itself cannot be examined at all; every
// other problem becomes an Errors or Notes entry so the report is
// complete, following packages.Validate's own collect-everything pattern.
func Validate(m BehavioralModel, inv inventory.Inventory) (ValidateReport, error) {
	var report ValidateReport

	if m.Schema != CurrentSchema {
		report.Errors = append(report.Errors, fmt.Sprintf("model declares schema %d, but this build only understands schema %d", m.Schema, CurrentSchema))
	}

	if got, want := m.SourceInventoryDigest, computeInventoryDigest(inv); got != want {
		report.Notes = append(report.Notes, fmt.Sprintf("model's source_inventory_digest (%s) differs from the supplied inventory's digest (%s) — model may have been built against a different inventory snapshot", got, want))
	}

	if len(m.Steps) == 0 {
		report.Errors = append(report.Errors, "model declares no steps")
	}

	entries := make(map[string]inventory.Entry, len(inv.Entries))
	for _, e := range inv.Entries {
		entries[e.Path] = e
	}
	checkEvidence := func(context string, refs []EvidenceRef) {
		if len(refs) == 0 {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: has no evidence", context))
			return
		}
		for _, ref := range refs {
			entry, found := entries[ref.Path]
			switch {
			case !found:
				report.Errors = append(report.Errors, fmt.Sprintf("%s: evidence cites %q, which is not in the supplied inventory", context, ref.Path))
			case entry.Digest != ref.Digest:
				report.Notes = append(report.Notes, fmt.Sprintf("%s: evidence for %q is stale (digest %s, inventory now has %s)", context, ref.Path, ref.Digest, entry.Digest))
			}
		}
	}

	stepIDs := make(map[string]bool)
	for _, step := range m.Steps {
		if stepIDs[step.ID] {
			report.Errors = append(report.Errors, fmt.Sprintf("duplicate step id %q", step.ID))
		}
		stepIDs[step.ID] = true

		checkEvidence(fmt.Sprintf("step %q", step.ID), step.Evidence)

		fields := map[string][]Claim{
			"inputs":     step.Inputs,
			"outputs":    step.Outputs,
			"skills":     step.Skills,
			"tools":      step.Tools,
			"references": step.References,
		}
		for _, field := range stepFieldsWithClaims {
			seen := make(map[string]bool)
			for _, claim := range fields[field] {
				if seen[claim.Value] {
					report.Errors = append(report.Errors, fmt.Sprintf("step %q: duplicate %s claim %q", step.ID, field, claim.Value))
				}
				seen[claim.Value] = true
				checkEvidence(fmt.Sprintf("step %q %s claim %q", step.ID, field, claim.Value), claim.Evidence)
			}
		}

		seenSetup := make(map[string]bool)
		for _, need := range step.Setup {
			if seenSetup[need.Path] {
				report.Errors = append(report.Errors, fmt.Sprintf("step %q: duplicate setup need %q", step.ID, need.Path))
			}
			seenSetup[need.Path] = true
			checkEvidence(fmt.Sprintf("step %q setup need %q", step.ID, need.Path), need.Evidence)
		}

		seenGates := make(map[string]bool)
		for _, gate := range step.Gates {
			if seenGates[gate.ID] {
				report.Errors = append(report.Errors, fmt.Sprintf("step %q: duplicate gate id %q", step.ID, gate.ID))
			}
			seenGates[gate.ID] = true
			checkEvidence(fmt.Sprintf("step %q gate %q", step.ID, gate.ID), gate.Evidence)
		}
	}

	seenDecisions := make(map[string]bool)
	for _, d := range m.Decisions {
		if seenDecisions[d.ID] {
			report.Errors = append(report.Errors, fmt.Sprintf("duplicate decision id %q", d.ID))
		}
		seenDecisions[d.ID] = true

		checkEvidence(fmt.Sprintf("decision %q", d.ID), d.Evidence)

		for _, ref := range d.Refs {
			if err := validateRef(ref, m); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("decision %q: %v", d.ID, err))
			}
		}
	}

	return report, nil
}

// stepFieldsWithClaims is the subset of stepFields backed by []Claim (all
// but "setup" and "gates", which have their own element types).
var stepFieldsWithClaims = []string{"inputs", "outputs", "skills", "tools", "references"}

// validateRef checks that ref resolves to something that actually exists
// in m: a real Step, and if Field/Key are set, a real item under that
// step's named field bearing that Key — catching a Ref naming the wrong
// field for where an item lives, not just a dangling key.
func validateRef(ref Ref, m BehavioralModel) error {
	if ref.StepID == "" {
		if ref.Field != "" || ref.Key != "" {
			return fmt.Errorf("ref has field/key %q/%q but no step_id", ref.Field, ref.Key)
		}
		return nil // model-level, always valid
	}

	var step *Step
	for i := range m.Steps {
		if m.Steps[i].ID == ref.StepID {
			step = &m.Steps[i]
			break
		}
	}
	if step == nil {
		return fmt.Errorf("ref names step %q, which does not exist", ref.StepID)
	}
	if ref.Field == "" {
		return nil // step-level
	}

	keys := fieldKeys(*step, ref.Field)
	if keys == nil {
		return fmt.Errorf("ref names unknown field %q", ref.Field)
	}
	if ref.Key == "" {
		return nil // field-level, no claim to point at yet
	}
	for _, k := range keys {
		if k == ref.Key {
			return nil
		}
	}
	return fmt.Errorf("ref names key %q in field %q of step %q, which does not exist there", ref.Key, ref.Field, ref.StepID)
}

// fieldKeys returns the natural identifiers held by step's named field —
// Claim.Value for claim-backed fields, SetupNeed.Path for "setup",
// Gate.ID for "gates" — or nil if field isn't a recognized name.
func fieldKeys(step Step, field string) []string {
	switch field {
	case "inputs":
		return claimValues(step.Inputs)
	case "outputs":
		return claimValues(step.Outputs)
	case "skills":
		return claimValues(step.Skills)
	case "tools":
		return claimValues(step.Tools)
	case "references":
		return claimValues(step.References)
	case "setup":
		keys := make([]string, len(step.Setup))
		for i, s := range step.Setup {
			keys[i] = s.Path
		}
		return keys
	case "gates":
		keys := make([]string, len(step.Gates))
		for i, g := range step.Gates {
			keys[i] = g.ID
		}
		return keys
	default:
		return nil
	}
}

func claimValues(claims []Claim) []string {
	values := make([]string, len(claims))
	for i, c := range claims {
		values[i] = c.Value
	}
	return values
}
