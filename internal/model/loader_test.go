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

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
