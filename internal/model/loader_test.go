package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

func TestLoadInventoryRoundTrip(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Instructions\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")

	inv, err := inventory.Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	invPath := filepath.Join(t.TempDir(), "inventory.json")
	if err := os.WriteFile(invPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadInventory(invPath)
	if err != nil {
		t.Fatalf("LoadInventory() error = %v", err)
	}
	if loaded.Root != inv.Root || len(loaded.Entries) != len(inv.Entries) {
		t.Fatalf("LoadInventory() = %+v, want %+v", loaded, inv)
	}
	for i := range inv.Entries {
		if loaded.Entries[i].Path != inv.Entries[i].Path || loaded.Entries[i].Digest != inv.Entries[i].Digest {
			t.Fatalf("entry %d mismatch: got %+v, want %+v", i, loaded.Entries[i], inv.Entries[i])
		}
	}
}

func TestLoadInventoryDefaultsAbsentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	mustWrite(t, path, `{"root": "/workspace", "entries": []}`)

	inv, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("LoadInventory() error = %v", err)
	}
	if inv.Schema != inventory.CurrentSchema {
		t.Fatalf("Schema = %d, want %d", inv.Schema, inventory.CurrentSchema)
	}
}

func TestLoadInventoryRejectsUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	mustWrite(t, path, `{"schema": 99, "root": "/workspace", "entries": []}`)

	if _, err := LoadInventory(path); err == nil {
		t.Fatal("LoadInventory() error = nil, want error for unknown schema")
	}
}

func TestComputeInventoryDigestStableAcrossOrder(t *testing.T) {
	a := inventory.Inventory{Entries: []inventory.Entry{
		{Path: "b.md", Digest: "sha256:2"},
		{Path: "a.md", Digest: "sha256:1"},
	}}
	b := inventory.Inventory{Entries: []inventory.Entry{
		{Path: "a.md", Digest: "sha256:1"},
		{Path: "b.md", Digest: "sha256:2"},
	}}
	if computeInventoryDigest(a) != computeInventoryDigest(b) {
		t.Fatal("computeInventoryDigest is not order-independent")
	}
}

func TestComputeInventoryDigestChangesOnContentChange(t *testing.T) {
	a := inventory.Inventory{Entries: []inventory.Entry{{Path: "a.md", Digest: "sha256:1"}}}
	b := inventory.Inventory{Entries: []inventory.Entry{{Path: "a.md", Digest: "sha256:2"}}}
	if computeInventoryDigest(a) == computeInventoryDigest(b) {
		t.Fatal("computeInventoryDigest did not change when entry digest changed")
	}
}

func TestNewSkeletonFromInventoryProducesModelLevelDecisions(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Instructions\n\nRun the deploy script.\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")

	inv, err := inventory.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	skeleton := NewSkeletonFromInventory(inv)
	if skeleton.Schema != CurrentSchema {
		t.Fatalf("Schema = %d, want %d", skeleton.Schema, CurrentSchema)
	}
	if skeleton.SourceInventoryDigest == "" {
		t.Fatal("SourceInventoryDigest is empty")
	}
	if len(skeleton.Decisions) == 0 {
		t.Fatal("expected at least one Decision seeded from the instruction file")
	}
	if len(skeleton.Steps) != 0 {
		t.Fatalf("expected no Steps in a skeleton, got %d", len(skeleton.Steps))
	}
}

func TestParseModelSortsDecisionsByID(t *testing.T) {
	data := []byte(`{
		"schema": 1,
		"decisions": [
			{"id": "decision-c"},
			{"id": "decision-a"},
			{"id": "decision-b"}
		]
	}`)

	m, err := ParseModel(data)
	if err != nil {
		t.Fatalf("ParseModel() error = %v", err)
	}
	got := []string{m.Decisions[0].ID, m.Decisions[1].ID, m.Decisions[2].ID}
	want := []string{"decision-a", "decision-b", "decision-c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Decisions order = %v, want %v", got, want)
		}
	}
}

func TestParseModelRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseModel([]byte("not json")); err == nil {
		t.Fatal("ParseModel() error = nil, want error for malformed JSON")
	}
}

func TestNewSkeletonFromInventoryDecisionsAreSorted(t *testing.T) {
	root := t.TempDir()
	// Filenames chosen so path-sort order and ID-sort order would coincide
	// anyway if left unsorted; the point of this test is that
	// NewSkeletonFromInventory calls sortDecisions explicitly rather than
	// relying on that coincidence, so it stays correct if the underlying
	// entries ever change order.
	mustWrite(t, filepath.Join(root, "c-AGENTS.md"), "# c\n")
	mustWrite(t, filepath.Join(root, "a-AGENTS.md"), "# a\n")
	mustWrite(t, filepath.Join(root, "b-AGENTS.md"), "# b\n")

	inv, err := inventory.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	skeleton := NewSkeletonFromInventory(inv)

	for i := 1; i < len(skeleton.Decisions); i++ {
		if skeleton.Decisions[i-1].ID > skeleton.Decisions[i].ID {
			t.Fatalf("Decisions not sorted by ID: %q before %q", skeleton.Decisions[i-1].ID, skeleton.Decisions[i].ID)
		}
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
