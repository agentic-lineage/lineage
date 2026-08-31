// Package graph records a project's local lineage graph: an append-only
// log of "this local environment descended from that package" events. It
// exists so a receiver (or Lineage itself) can later explain where a
// project's enabled state came from, without needing a snapshot store or a
// session identity system to exist first (see docs/decisions/0013).
package graph

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-lineage/lineage/internal/atomicfile"
)

const (
	stateDirName = ".lineage"
	fileName     = "graph.json"
)

// ParentRef identifies the package a descendant environment came from,
// using the same identity Lineage uses everywhere else (name, version, and
// the sha256 content digest from packages.ComputeDigest — see
// docs/decisions/0005). SnapshotID is reserved for issue #7's future
// content-addressed snapshot store and stays empty until that exists.
type ParentRef struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Digest     string `json:"digest,omitempty"`
	SnapshotID string `json:"snapshot_id,omitempty"`
}

// DescendantRef identifies the local environment a record describes.
// SessionID is reserved for a future per-run session identity — no such
// concept exists in the runtime yet — and stays empty until it does.
type DescendantRef struct {
	Workspace string `json:"workspace,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// Record is one entry in a project's local lineage graph. Provider is
// deliberately a separate, plain string map from Parent/Descendant: it
// carries optional adapter-specific metadata without ever mixing into the
// core provider-neutral identity fields. No field on Record (or anything
// it embeds) holds transcript or message content — the schema simply has
// nowhere to put it.
type Record struct {
	ID         string            `json:"id"`
	CreatedAt  time.Time         `json:"created_at"`
	Event      string            `json:"event"`
	Parent     ParentRef         `json:"parent"`
	Descendant DescendantRef     `json:"descendant"`
	Provider   map[string]string `json:"provider,omitempty"`
}

func statePath(projectRoot string) string {
	return filepath.Join(projectRoot, stateDirName, fileName)
}

// Path returns where a project's local lineage graph file lives, for
// callers (such as `lineage doctor`) that need to name it in diagnostic
// output without reaching into package-private layout details.
func Path(projectRoot string) string {
	return statePath(projectRoot)
}

// newID returns an opaque, unique record identifier: 16 random bytes,
// hex-encoded. It carries no meaning beyond identity, unlike package
// identity (name/version/digest) which is meaningful on its own.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate record id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Append records a new descendant event for projectRoot. If rec.ID or
// rec.CreatedAt are unset, they're filled in before saving. The record
// actually written (with those fields populated) is returned so callers
// don't have to reconstruct it.
func Append(projectRoot string, rec Record) (Record, error) {
	if rec.ID == "" {
		id, err := newID()
		if err != nil {
			return Record{}, err
		}
		rec.ID = id
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}

	records, err := Load(projectRoot)
	if err != nil {
		return Record{}, err
	}
	records = append(records, rec)

	if err := save(projectRoot, records); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// Load returns every record in projectRoot's local lineage graph, in the
// order they were appended. A project with no graph file yet (nothing has
// been recorded) returns an empty slice, not an error.
func Load(projectRoot string) ([]Record, error) {
	path := statePath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return records, nil
}

// ForPackage returns every record whose Parent.Name matches name, in the
// order they were appended.
func ForPackage(projectRoot, name string) ([]Record, error) {
	records, err := Load(projectRoot)
	if err != nil {
		return nil, err
	}
	matches := make([]Record, 0, len(records))
	for _, rec := range records {
		if rec.Parent.Name == name {
			matches = append(matches, rec)
		}
	}
	return matches, nil
}

func save(projectRoot string, records []Record) error {
	path := statePath(projectRoot)
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0o644)
}
