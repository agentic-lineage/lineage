package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/agentic-lineage/lineage/internal/inventory"
)

// LoadInventory reads and parses a stored inventory.Inventory JSON file.
// Nothing in internal/inventory reads a stored inventory back yet — its
// own doc comment flags this as a gap for "whoever adds the first reader"
// to close, following packages.LoadManifest's schema-defaulting pattern.
// This is that first reader: an absent or zero schema field is treated as
// inventory.CurrentSchema (the only schema that ever existed before this
// reader existed), and any other value is rejected outright rather than
// silently misparsed, using the same probe-pointer trick LoadManifest uses
// to tell "field absent" apart from "field explicitly zero".
func LoadInventory(path string) (inventory.Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return inventory.Inventory{}, fmt.Errorf("read inventory %s: %w", path, err)
	}

	var inv inventory.Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return inventory.Inventory{}, fmt.Errorf("parse inventory %s: %w", path, err)
	}

	var probe struct {
		Schema *int `json:"schema"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return inventory.Inventory{}, fmt.Errorf("parse inventory %s: %w", path, err)
	}
	if probe.Schema == nil {
		inv.Schema = inventory.CurrentSchema
	}
	if inv.Schema != inventory.CurrentSchema {
		return inventory.Inventory{}, fmt.Errorf("inventory %s declares schema %d, but this build only understands schema %d", path, inv.Schema, inventory.CurrentSchema)
	}

	return inv, nil
}

// computeInventoryDigest hashes inv.Entries sorted by Path, writing each
// entry's Path then its already-computed Digest — mirroring
// packages.ComputeDigest's "hash sorted (path, content) pairs" shape, one
// level of indirection up: an inventory.Entry.Digest already is that
// file's content hash, so there is no need to re-read the file itself.
// The result changes if any entry's content digest changes, or if the set
// of entries itself changes (a file added or removed).
func computeInventoryDigest(inv inventory.Inventory) string {
	entries := append([]inventory.Entry(nil), inv.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e.Path))
		h.Write([]byte(e.Digest))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// NewSkeletonFromInventory produces a minimal, mechanically-derived
// BehavioralModel skeleton from inv — not semantic modeling, that belongs
// to a later, agent-assisted analysis stage. It creates one model-level
// Decision per instruction-kind or unclassified-markdown entry (the
// content most likely to describe workflow steps this package cannot
// itself understand), each carrying that entry as evidence, and leaves
// Steps empty for a human or later stage to fill in. Documented explicitly
// as heuristic scaffolding, matching classify.go's own "does not
// interpret" boundary.
func NewSkeletonFromInventory(inv inventory.Inventory) BehavioralModel {
	m := BehavioralModel{
		Schema:                CurrentSchema,
		SourceInventoryDigest: computeInventoryDigest(inv),
	}

	for _, e := range inv.Entries {
		if e.Kind != inventory.KindInstruction && e.Kind != inventory.KindUnknown {
			continue
		}
		if e.Kind == inventory.KindUnknown && e.Language != "markdown" {
			continue
		}
		m.Decisions = append(m.Decisions, Decision{
			ID:          "decision-" + e.Path,
			Description: fmt.Sprintf("%s (%s: %s) may describe workflow steps not yet modeled", e.Path, e.Kind, e.Reason),
			Evidence: []EvidenceRef{
				{Path: e.Path, Digest: e.Digest},
			},
		})
	}

	return m
}
