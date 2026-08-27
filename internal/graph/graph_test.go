package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOnEmptyProjectReturnsEmptySlice(t *testing.T) {
	root := t.TempDir()

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Load() = %d records on a project with no graph file, want 0", len(records))
	}
}

func TestAppendFillsIDAndCreatedAt(t *testing.T) {
	root := t.TempDir()

	rec, err := Append(root, Record{
		Event:  "enable",
		Parent: ParentRef{Name: "review-pack", Version: "1.0.0", Digest: "sha256:abc"},
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if rec.ID == "" {
		t.Fatal("Append() left ID empty")
	}
	if rec.CreatedAt.IsZero() {
		t.Fatal("Append() left CreatedAt zero")
	}
}

func TestAppendPreservesCallerSuppliedIDAndCreatedAt(t *testing.T) {
	root := t.TempDir()

	first, err := Append(root, Record{
		ID:     "fixed-id",
		Event:  "enable",
		Parent: ParentRef{Name: "review-pack", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if first.ID != "fixed-id" {
		t.Fatalf("Append() ID = %q, want caller-supplied %q", first.ID, "fixed-id")
	}
}

func TestAppendIsCumulative(t *testing.T) {
	root := t.TempDir()

	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "a", Version: "1.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "b", Version: "1.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Load() = %d records after two Append calls, want 2", len(records))
	}
	if records[0].Parent.Name != "a" || records[1].Parent.Name != "b" {
		t.Fatalf("Load() order = %q, %q, want a, b (append order preserved)", records[0].Parent.Name, records[1].Parent.Name)
	}
}

func TestAppendGeneratesDistinctIDs(t *testing.T) {
	root := t.TempDir()

	first, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "a", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	second, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "b", Version: "1.0.0"}})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("Append() generated the same ID %q twice, want distinct IDs", first.ID)
	}
}

func TestForPackageFiltersByParentName(t *testing.T) {
	root := t.TempDir()

	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "review-pack", Version: "1.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "other-pack", Version: "1.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "review-pack", Version: "2.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	matches, err := ForPackage(root, "review-pack")
	if err != nil {
		t.Fatalf("ForPackage() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("ForPackage() = %d records, want 2", len(matches))
	}
	for _, rec := range matches {
		if rec.Parent.Name != "review-pack" {
			t.Fatalf("ForPackage(%q) returned record for %q", "review-pack", rec.Parent.Name)
		}
	}
}

func TestForPackageNoMatchesReturnsEmptySlice(t *testing.T) {
	root := t.TempDir()

	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "review-pack", Version: "1.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	matches, err := ForPackage(root, "no-such-package")
	if err != nil {
		t.Fatalf("ForPackage() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("ForPackage() = %d records, want 0", len(matches))
	}
}

func TestRecordRoundTripsThroughJSON(t *testing.T) {
	root := t.TempDir()

	written, err := Append(root, Record{
		Event: "enable",
		Parent: ParentRef{
			Name:    "review-pack",
			Version: "1.0.0",
			Digest:  "sha256:abc123",
		},
		Descendant: DescendantRef{
			Workspace: "team-workspace",
		},
		Provider: map[string]string{"provider": "claude"},
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	records, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Load() = %d records, want 1", len(records))
	}
	got := records[0]

	if got.ID != written.ID {
		t.Errorf("ID = %q, want %q", got.ID, written.ID)
	}
	if !got.CreatedAt.Equal(written.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, written.CreatedAt)
	}
	if got.Event != "enable" {
		t.Errorf("Event = %q, want %q", got.Event, "enable")
	}
	if got.Parent != written.Parent {
		t.Errorf("Parent = %+v, want %+v", got.Parent, written.Parent)
	}
	if got.Descendant != written.Descendant {
		t.Errorf("Descendant = %+v, want %+v", got.Descendant, written.Descendant)
	}
	if got.Provider["provider"] != "claude" {
		t.Errorf("Provider[\"provider\"] = %q, want %q", got.Provider["provider"], "claude")
	}
}

func TestAppendWritesReadableJSONFile(t *testing.T) {
	root := t.TempDir()

	if _, err := Append(root, Record{Event: "enable", Parent: ParentRef{Name: "review-pack", Version: "1.0.0"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	path := filepath.Join(root, ".lineage", "graph.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("graph.json is not valid JSON: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("graph.json has %d entries, want 1", len(raw))
	}
}

func TestLoadRejectsCorruptFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".lineage"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".lineage", "graph.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Load(root); err == nil {
		t.Fatal("Load() error = nil for a corrupt graph.json, want error")
	}
}
