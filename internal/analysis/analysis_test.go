package analysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/inventory"
	"github.com/agentic-lineage/lineage/internal/model"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// smallWorkspace builds a two-file workspace (one instruction file citing
// one script) and returns its discovered inventory, following
// internal/model's own inline-fixture-builder convention rather than a
// testdata/ directory.
func smallWorkspace(t *testing.T) inventory.Inventory {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Instructions\n\nRun scripts/deploy.sh to deploy.\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")
	inv, err := inventory.Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	return inv
}

func entryDigest(t *testing.T, inv inventory.Inventory, path string) string {
	t.Helper()
	for _, e := range inv.Entries {
		if e.Path == path {
			return e.Digest
		}
	}
	t.Fatalf("no entry for %q", path)
	return ""
}

// validModelFor returns a BehavioralModel that cites inv's real paths and
// digests, so a test can mutate exactly one thing at a time from a known
// good state.
func validModelFor(t *testing.T, inv inventory.Inventory) model.BehavioralModel {
	t.Helper()
	claudeDigest := entryDigest(t, inv, "CLAUDE.md")
	deployDigest := entryDigest(t, inv, "scripts/deploy.sh")
	return model.BehavioralModel{
		Schema: model.CurrentSchema,
		Name:   "deploy",
		Intent: "deploy the app",
		Steps: []model.Step{
			{
				ID:          "step-1-deploy",
				Name:        "Deploy",
				Description: "Run the deploy script",
				Tools: []model.Claim{
					{Value: "scripts/deploy.sh", Evidence: []model.EvidenceRef{
						{Path: "scripts/deploy.sh", Digest: deployDigest, Note: "the deploy tool"},
					}},
				},
				Evidence: []model.EvidenceRef{
					{Path: "CLAUDE.md", Digest: claudeDigest, Line: 3, Note: "Run scripts/deploy.sh to deploy."},
				},
			},
		},
	}
}

func toJSON(t *testing.T, v any) []byte {
	t.Helper()
	// SourceInventoryDigest is computed against a specific inventory
	// snapshot by the caller before marshaling, same as production
	// providers would need to when told the source_inventory_digest to echo
	// back in their prompt.
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return data
}

func withDigest(t *testing.T, inv inventory.Inventory, m model.BehavioralModel) model.BehavioralModel {
	t.Helper()
	// Mirrors model.NewSkeletonFromInventory: SourceInventoryDigest must
	// match inv exactly for Validate to accept it. Computed the same way
	// loader.go does, via a round trip through ParseModel of a skeleton so
	// the test doesn't need to reach into model's unexported
	// computeInventoryDigest.
	m.SourceInventoryDigest = model.NewSkeletonFromInventory(inv).SourceInventoryDigest
	return m
}

func TestAnalyzeSuccess(t *testing.T) {
	inv := smallWorkspace(t)
	m := withDigest(t, inv, validModelFor(t, inv))
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !result.Report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", result.Report.Errors)
	}
}

func TestAnalyzeRejectsMalformedJSON(t *testing.T) {
	inv := smallWorkspace(t)
	p := FixtureProvider{Response: []byte("not json")}

	_, err := Analyze(context.Background(), p, inv)
	if err == nil {
		t.Fatal("Analyze() error = nil, want error for malformed provider output")
	}
}

func TestAnalyzeFailsClosedOnWrongSchema(t *testing.T) {
	inv := smallWorkspace(t)
	m := withDigest(t, inv, validModelFor(t, inv))
	m.Schema = 99
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Report.Passed() {
		t.Fatal("report.Passed() = true, want false for schema mismatch")
	}
	assertErrorContains(t, result.Report.Errors, "schema 99")
}

func TestAnalyzeFailsClosedOnMissingEvidence(t *testing.T) {
	inv := smallWorkspace(t)
	m := withDigest(t, inv, validModelFor(t, inv))
	m.Steps[0].Evidence[0].Path = "does/not/exist.md"
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Report.Passed() {
		t.Fatal("report.Passed() = true, want false for evidence not in inventory")
	}
	assertErrorContains(t, result.Report.Errors, "does/not/exist.md")
}

func TestAnalyzeFailsClosedOnStaleEvidence(t *testing.T) {
	inv := smallWorkspace(t)
	m := withDigest(t, inv, validModelFor(t, inv))
	m.Steps[0].Evidence[0].Digest = "sha256:stale"
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Report.Passed() {
		t.Fatal("report.Passed() = true, want false for stale evidence digest")
	}
	assertErrorContains(t, result.Report.Errors, "stale")
}

// TestAnalyzeSurfacesConflictingInstructionsAsDecision builds a workspace
// from two instruction files that disagree about which script to run for
// the same step, and asserts a provider response that reports this as an
// unresolved Decision (rather than silently picking one) is accepted -
// exactly the "surface ambiguity as decisions, not silently filled in"
// requirement from #104's acceptance criteria.
func TestAnalyzeSurfacesConflictingInstructionsAsDecision(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Instructions\n\nRun scripts/deploy.sh to deploy.\n")
	mustWrite(t, filepath.Join(root, "SKILL.md"), "# Skill\n\nRun scripts/release.sh to deploy.\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")
	mustWrite(t, filepath.Join(root, "scripts", "release.sh"), "#!/bin/sh\necho release\n")
	inv, err := inventory.Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	claudeDigest := entryDigest(t, inv, "CLAUDE.md")
	skillDigest := entryDigest(t, inv, "SKILL.md")

	m := model.BehavioralModel{
		Schema: model.CurrentSchema,
		Name:   "deploy",
		Intent: "deploy the app",
		Steps: []model.Step{
			{
				ID:          "step-1-deploy",
				Name:        "Deploy",
				Description: "Deploy the app (which script is unclear)",
				Evidence: []model.EvidenceRef{
					{Path: "CLAUDE.md", Digest: claudeDigest, Line: 3},
				},
			},
		},
		Decisions: []model.Decision{
			{
				ID:          "decision-deploy-script",
				Description: "CLAUDE.md says deploy.sh, SKILL.md says release.sh for the same deploy step - which is authoritative?",
				Refs:        []model.Ref{{StepID: "step-1-deploy", Field: "tools"}},
				Evidence: []model.EvidenceRef{
					{Path: "CLAUDE.md", Digest: claudeDigest, Line: 3, Note: "Run scripts/deploy.sh to deploy."},
					{Path: "SKILL.md", Digest: skillDigest, Line: 3, Note: "Run scripts/release.sh to deploy."},
				},
			},
		},
	}
	m = withDigest(t, inv, m)
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !result.Report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v (an unresolved Decision must be valid, not rejected)", result.Report.Errors)
	}
	if len(result.Model.Decisions) != 1 {
		t.Fatalf("len(Decisions) = %d, want 1", len(result.Model.Decisions))
	}
}

func TestAnalyzeNotesLowConfidenceEvidence(t *testing.T) {
	inv := smallWorkspace(t)
	m := validModelFor(t, inv)
	m.Steps[0].Evidence[0].Note = "" // strip the quote a real provider should always include
	m = withDigest(t, inv, m)
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	assertNoteContains(t, result.Report.Notes, "no supporting note/quote")
}

func TestAnalyzeNotesHighDecisionRatio(t *testing.T) {
	inv := smallWorkspace(t)
	m := withDigest(t, inv, validModelFor(t, inv))
	claudeDigest := entryDigest(t, inv, "CLAUDE.md")
	for i := 0; i < 3; i++ {
		m.Decisions = append(m.Decisions, model.Decision{
			ID:          "decision-" + string(rune('a'+i)),
			Description: "padding",
			Evidence:    []model.EvidenceRef{{Path: "CLAUDE.md", Digest: claudeDigest, Note: "x"}},
		})
	}
	p := FixtureProvider{Response: toJSON(t, m)}

	result, err := Analyze(context.Background(), p, inv)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	assertNoteContains(t, result.Report.Notes, "decision-to-step ratio is high")
}

func TestAnalyzeWrapsProviderError(t *testing.T) {
	inv := smallWorkspace(t)
	p := FixtureProvider{Err: context.DeadlineExceeded}

	_, err := Analyze(context.Background(), p, inv)
	if err == nil {
		t.Fatal("Analyze() error = nil, want provider error wrapped")
	}
}

func assertErrorContains(t *testing.T, errs []string, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return
		}
	}
	t.Fatalf("errors %#v do not contain %q", errs, substr)
}

func assertNoteContains(t *testing.T, notes []string, substr string) {
	t.Helper()
	for _, n := range notes {
		if strings.Contains(n, substr) {
			return
		}
	}
	t.Fatalf("notes %#v do not contain %q", notes, substr)
}
