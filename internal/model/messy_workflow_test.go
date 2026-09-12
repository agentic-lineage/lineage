package model

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

// buildDeployWorkspace lays out the fixture behind this package's worked
// example: a CLAUDE.md whose second line names no file ("Run the deploy
// script"), exactly the case inventory's own doc comment flags as beyond
// literal citation matching ("catches 'run scripts/deploy.sh' but not 'run
// the deploy script' with no filename in it"), plus a plausible but
// unconfirmed candidate script and an unrelated second script to make the
// candidate genuinely ambiguous rather than the only file in the tree.
func buildDeployWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"),
		"1. Run tests: `npm test`\n2. Run the deploy script\n3. Notify #releases in Slack\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")
	mustWrite(t, filepath.Join(root, "bin", "deploy"), "#!/bin/sh\necho deploy\n")
	return root
}

// TestMessyWorkflowRepresentableWithoutProviderFields is the acceptance
// criterion "a sample messy workflow can be represented without
// provider-specific fields" made concrete: build the deploy-script
// fixture, run inventory.Discover, hand-construct the BehavioralModel a
// human or a later agent-assisted stage would produce for it (the Tools
// claim stays empty because no evidence names a file; the ambiguity
// becomes a step-scoped Decision, per this package's worked example),
// Validate it clean, and reflect-walk the struct definitions to assert no
// field name or JSON tag anywhere names a provider.
func TestMessyWorkflowRepresentableWithoutProviderFields(t *testing.T) {
	root := buildDeployWorkspace(t)
	inv, err := inventory.Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	entry := func(path string) inventory.Entry {
		for _, e := range inv.Entries {
			if e.Path == path {
				return e
			}
		}
		t.Fatalf("fixture missing expected entry %q", path)
		return inventory.Entry{}
	}
	claudeMD := entry("CLAUDE.md")
	deploySH := entry("scripts/deploy.sh")

	m := BehavioralModel{
		Schema:                CurrentSchema,
		Name:                  "deploy-workflow",
		Intent:                "Run tests, deploy, notify",
		SourceInventoryDigest: computeInventoryDigest(inv),
		Steps: []Step{
			{
				ID:          "step-2-deploy",
				Name:        "Deploy",
				Description: "Run the deploy script (source text does not name a file)",
				Evidence:    []EvidenceRef{{Path: claudeMD.Path, Digest: claudeMD.Digest, Line: 2}},
			},
		},
		Decisions: []Decision{
			{
				ID:          "decision-1",
				Refs:        []Ref{{StepID: "step-2-deploy", Field: "tools"}},
				Description: `"the deploy script" is not a literal filename anywhere in the workspace's citations. Candidate: scripts/deploy.sh — plausible, unconfirmed.`,
				Evidence: []EvidenceRef{
					{Path: claudeMD.Path, Digest: claudeMD.Digest, Line: 2},
					{Path: deploySH.Path, Digest: deploySH.Digest, Note: "candidate, no citation names it"},
				},
			},
		},
	}

	report, err := Validate(m, inv)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !report.Passed() {
		t.Fatalf("report.Passed() = false, errors = %#v", report.Errors)
	}

	assertNoProviderFields(t, reflect.TypeOf(BehavioralModel{}))
}

// providerTokens names the provider identifiers this package must never
// bake into its own schema — Manifest.Entrypoints is the one place those
// belong, downstream of this model, at #106's compilation stage.
var providerTokens = []string{"claude", "codex", "cursor", "aider", "cline", "windsurf", "auggie"}

func assertNoProviderFields(t *testing.T, typ reflect.Type) {
	t.Helper()
	seen := make(map[reflect.Type]bool)
	walkType(t, typ, seen)
}

func walkType(t *testing.T, typ reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		lower := strings.ToLower(field.Name)
		tag := strings.ToLower(field.Tag.Get("json"))
		for _, token := range providerTokens {
			if strings.Contains(lower, token) || strings.Contains(tag, token) {
				t.Errorf("field %s.%s (json tag %q) names provider token %q — model must stay provider-neutral", typ.Name(), field.Name, tag, token)
			}
		}
		walkType(t, field.Type, seen)
	}
}
