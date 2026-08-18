// Package materialize stages enabled packages' content where a wrapped
// agent provider actually reads it, instead of leaving discovery results
// stranded in `lineage run --dry-run` output. It is provider-neutral: all
// provider-specific paths come from a provider.Provider value supplied by the
// caller.
package materialize

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lineage-dev/lineage/internal/packages"
	"github.com/lineage-dev/lineage/internal/provider"
)

const (
	stateDirName = ".lineage"
	beginMarker  = "<!-- lineage:begin -->"
	endMarker    = "<!-- lineage:end -->"
)

// state is the record of exactly what the last Apply call wrote for one
// provider, so a later call can remove entries that are no longer desired
// (a package got disabled, a skill got removed) instead of only ever adding.
type state struct {
	SkillDirs []string `json:"skill_dirs"` // relative to project root, sorted
}

func statePath(projectRoot, providerName string) string {
	return filepath.Join(projectRoot, stateDirName, "materialized-"+providerName+".json")
}

// Apply stages every skill from pkgs into adapter.SkillsDir and refreshes
// the generated section of adapter.ContextFile describing active packages,
// agents, policies, and workflows.
//
// It is idempotent and reversible: calling it again with a different (or
// empty) set of packages removes whatever a previous call staged that is no
// longer desired. This is tracked via a small per-provider state file
// (.lineage/materialized-<provider>.json) rather than guessed from disk
// contents, so cleanup is exact even if the package set shrinks between
// runs.
func Apply(projectRoot string, adapter provider.Provider, pkgs []packages.Package) error {
	prev, err := loadState(projectRoot, adapter.Name)
	if err != nil {
		return err
	}

	desired := desiredSkillDirs(adapter, pkgs)

	for _, rel := range prev.SkillDirs {
		if _, ok := desired[rel]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(projectRoot, rel)); err != nil {
			return fmt.Errorf("remove stale materialized skill %s: %w", rel, err)
		}
	}

	written := make([]string, 0, len(desired))
	for rel, src := range desired {
		dest := filepath.Join(projectRoot, rel)
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("clear %s before staging: %w", rel, err)
		}
		if err := copyDir(src, dest); err != nil {
			return fmt.Errorf("stage skill into %s: %w", rel, err)
		}
		written = append(written, rel)
	}
	sort.Strings(written)

	if err := writeSummary(filepath.Join(projectRoot, adapter.ContextFile), pkgs); err != nil {
		return fmt.Errorf("update %s: %w", adapter.ContextFile, err)
	}

	return saveState(projectRoot, adapter.Name, state{SkillDirs: written})
}

// NeedsApproval reports whether calling Apply with pkgs would change
// anything on disk for this provider — a first-time materialization, or a
// package/skill set that differs from what the last Apply call recorded.
// It's used to gate materialization behind an explicit confirmation
// without re-prompting on every run once a given package set has already
// been approved and materialized.
func NeedsApproval(projectRoot string, adapter provider.Provider, pkgs []packages.Package) (bool, error) {
	prev, err := loadState(projectRoot, adapter.Name)
	if err != nil {
		return false, err
	}

	desired := desiredSkillDirs(adapter, pkgs)
	desiredDirs := make([]string, 0, len(desired))
	for rel := range desired {
		desiredDirs = append(desiredDirs, rel)
	}
	sort.Strings(desiredDirs)

	prevDirs := append([]string(nil), prev.SkillDirs...)
	sort.Strings(prevDirs)

	return !equalStrings(desiredDirs, prevDirs), nil
}

func desiredSkillDirs(adapter provider.Provider, pkgs []packages.Package) map[string]string {
	desired := map[string]string{} // relative skill dir -> absolute source dir
	for _, pkg := range pkgs {
		for _, skill := range pkg.Skills {
			rel := filepath.Join(adapter.SkillsDir, pkg.Manifest.Name+"-"+skill)
			desired[rel] = filepath.Join(pkg.Path, "skills", skill)
		}
	}
	return desired
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func loadState(projectRoot, providerName string) (state, error) {
	data, err := os.ReadFile(statePath(projectRoot, providerName))
	if err != nil {
		if os.IsNotExist(err) {
			return state{}, nil
		}
		return state{}, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return state{}, fmt.Errorf("parse %s: %w", statePath(projectRoot, providerName), err)
	}
	return s, nil
}

func saveState(projectRoot, providerName string, s state) error {
	path := statePath(projectRoot, providerName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeSummary(path string, pkgs []packages.Package) error {
	block := renderSummaryBlock(pkgs)

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	next := block
	if err == nil {
		next = replaceBlock(string(existing), block)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(next), 0o644)
}

func renderSummaryBlock(pkgs []packages.Package) string {
	var b strings.Builder
	b.WriteString(beginMarker + "\n")
	b.WriteString("Lineage-managed section. Do not edit by hand; it is regenerated on every `lineage run`.\n\n")
	b.WriteString("Active Lineage packages:\n\n")
	if len(pkgs) == 0 {
		b.WriteString("none\n")
	}
	for _, pkg := range pkgs {
		fmt.Fprintf(&b, "- %s@%s", pkg.Manifest.Name, pkg.Manifest.Version)
		if pkg.Manifest.Description != "" {
			fmt.Fprintf(&b, " - %s", pkg.Manifest.Description)
		}
		b.WriteString("\n")
		if len(pkg.Agents) > 0 {
			fmt.Fprintf(&b, "  agents: %s\n", strings.Join(pkg.Agents, ", "))
		}
		if len(pkg.Policies) > 0 {
			fmt.Fprintf(&b, "  policies: %s\n", strings.Join(pkg.Policies, ", "))
		}
		if len(pkg.Workflows) > 0 {
			fmt.Fprintf(&b, "  workflows: %s\n", strings.Join(pkg.Workflows, ", "))
		}
	}
	b.WriteString(endMarker + "\n")
	return b.String()
}

// replaceBlock swaps the content between beginMarker and endMarker for
// block, appending block to the end of existing content if no markers are
// present yet.
func replaceBlock(existing, block string) string {
	beginIdx := strings.Index(existing, beginMarker)
	endIdx := strings.Index(existing, endMarker)
	if beginIdx == -1 || endIdx == -1 || endIdx < beginIdx {
		trimmed := strings.TrimRight(existing, "\n")
		if trimmed == "" {
			return block
		}
		return trimmed + "\n\n" + block
	}
	after := strings.TrimPrefix(existing[endIdx+len(endMarker):], "\n")
	return existing[:beginIdx] + block + after
}

func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to materialize symlink %s", path)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
