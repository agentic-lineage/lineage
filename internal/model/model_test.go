package model

import (
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

// validInventory returns a small inventory containing exactly the two
// files validModel's evidence cites, with digests matching what validModel
// expects, so tests can start from a state that passes cleanly and then
// mutate one thing at a time.
func validInventory() inventory.Inventory {
	entries := []inventory.Entry{
		{Path: "CLAUDE.md", Digest: "sha256:aaaa"},
		{Path: "scripts/deploy.sh", Digest: "sha256:bbbb"},
	}
	return inventory.Inventory{Schema: inventory.CurrentSchema, Root: "/workspace", Entries: entries}
}

func validModel(inv inventory.Inventory) BehavioralModel {
	return BehavioralModel{
		Schema:                CurrentSchema,
		Name:                  "deploy",
		Intent:                "test and deploy",
		SourceInventoryDigest: computeInventoryDigest(inv),
		Steps: []Step{
			{
				ID:          "step-1-deploy",
				Name:        "Deploy",
				Description: "Run the deploy script",
				Tools: []Claim{
					{Value: "scripts/deploy.sh", Evidence: []EvidenceRef{{Path: "scripts/deploy.sh", Digest: "sha256:bbbb"}}},
				},
				Evidence: []EvidenceRef{{Path: "CLAUDE.md", Digest: "sha256:aaaa", Line: 1}},
			},
		},
	}
}

func TestValidateAcceptsValidModel(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)

	report, err := Validate(m, inv)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", report.Errors)
	}
}

func TestValidateRejectsBadSchema(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Schema = 99

	report, _ := Validate(m, inv)
	assertErrorContains(t, report, "schema 99")
}

func TestValidateRejectsEmptySteps(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Steps = nil

	report, _ := Validate(m, inv)
	assertErrorContains(t, report, "no steps")
}

func TestValidateRejectsDanglingEvidenceDigest(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Steps[0].Evidence[0].Digest = "sha256:stale"

	report, _ := Validate(m, inv)
	// Staleness is a Note, not an Error: the file still exists and the
	// model is still structurally valid, it's just built from an older
	// snapshot — Passed() should stay true.
	if !report.Passed() {
		t.Fatalf("expected Passed() = true (staleness is a Note, not an Error), got errors = %#v", report.Errors)
	}
	found := false
	for _, n := range report.Notes {
		if strings.Contains(n, "stale") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a staleness note, got errors=%#v notes=%#v", report.Errors, report.Notes)
	}
}

func TestValidateRejectsEvidenceNotInInventory(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Steps[0].Evidence[0].Path = "does/not/exist.md"

	report, _ := Validate(m, inv)
	assertErrorContains(t, report, "does/not/exist.md")
}

func TestValidateRejectsClaimWithNoEvidence(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Steps[0].Tools[0].Evidence = nil

	report, _ := Validate(m, inv)
	assertErrorContains(t, report, "has no evidence")
}

func TestValidateRejectsDuplicateClaimValue(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Steps[0].Tools = append(m.Steps[0].Tools, m.Steps[0].Tools[0])

	report, _ := Validate(m, inv)
	assertErrorContains(t, report, "duplicate")
}

func TestValidateRejectsRefNamingWrongField(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Decisions = []Decision{
		{
			ID:          "decision-1",
			Description: "wrong field for this key",
			Refs:        []Ref{{StepID: "step-1-deploy", Field: "skills", Key: "scripts/deploy.sh"}},
		},
	}

	report, _ := Validate(m, inv)
	assertErrorContains(t, report, "skills")
}

func TestValidateAcceptsModelLevelDecision(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Decisions = []Decision{
		{
			ID:          "decision-1",
			Description: "workspace-wide ambiguity, no single step",
			Evidence:    []EvidenceRef{{Path: "CLAUDE.md", Digest: "sha256:aaaa"}},
		},
	}

	report, err := Validate(m, inv)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", report.Errors)
	}
}

func TestValidateAcceptsFieldLevelRefWithNoClaimYet(t *testing.T) {
	inv := validInventory()
	m := validModel(inv)
	m.Decisions = []Decision{
		{
			ID:          "decision-1",
			Description: "this step's skills are unresolved",
			Refs:        []Ref{{StepID: "step-1-deploy", Field: "skills"}},
			Evidence:    []EvidenceRef{{Path: "CLAUDE.md", Digest: "sha256:aaaa"}},
		},
	}

	report, err := Validate(m, inv)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", report.Errors)
	}
}

func assertErrorContains(t *testing.T, report ValidateReport, substr string) {
	t.Helper()
	if report.Passed() {
		t.Fatalf("expected an error containing %q, but report passed", substr)
	}
	for _, e := range report.Errors {
		if strings.Contains(e, substr) {
			return
		}
	}
	t.Fatalf("expected an error containing %q, got %#v", substr, report.Errors)
}
