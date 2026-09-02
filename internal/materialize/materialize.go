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

	"github.com/agentic-lineage/lineage/internal/atomicfile"
	"github.com/agentic-lineage/lineage/internal/packages"
	"github.com/agentic-lineage/lineage/internal/provider"
)

const (
	stateDirName = ".lineage"
	beginMarker  = "<!-- lineage:begin -->"
	endMarker    = "<!-- lineage:end -->"
)

// currentStateSchema is the only materialized-<provider>.json schema this
// build understands. Mirrors packages.CurrentSchema and
// config.CurrentConfigSchema (see docs/decisions/0005): a future
// incompatible change to this file's shape has a clean way to say so
// instead of a parser guessing.
const currentStateSchema = 1

// state is the record of exactly what the last Apply call wrote for one
// provider, so a later call can remove entries that are no longer desired
// (a package got disabled, a skill got removed) instead of only ever adding.
type state struct {
	Schema    int      `json:"schema"`
	SkillDirs []string `json:"skill_dirs"` // relative to project root, sorted
}

func statePath(projectRoot, providerName string) string {
	return filepath.Join(projectRoot, stateDirName, "materialized-"+providerName+".json")
}

// HasState reports whether a provider has ever been materialized for this
// project — i.e. whether Apply has run for it before. Used to decide which
// providers need re-materializing after a package is disabled, without
// creating a state file for a provider that was never used.
func HasState(projectRoot, providerName string) (bool, error) {
	_, err := os.Stat(statePath(projectRoot, providerName))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
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
	return apply(projectRoot, adapter, pkgs, nil)
}

// WorkflowSequence describes an active workflow's ordered steps, so the
// generated context file can say "follow these skills in this order"
// instead of just listing an unordered package/skill set.
type WorkflowSequence struct {
	Name  string
	Steps []string
}

// ApplyWorkflow stages only pkg's workflow steps (not its full skill set)
// into adapter.SkillsDir, in the same idempotent/reversible way Apply does,
// and writes the generated context file with the workflow's ordered
// sequence made explicit. Scoping to just the workflow's steps is what
// makes `lineage workflow run` meaningfully different from enabling the
// whole package and running normally: only the skills the workflow
// actually references get materialized.
func ApplyWorkflow(projectRoot string, adapter provider.Provider, pkg packages.Package, wf packages.Workflow) error {
	scoped := pkg
	scoped.Skills = append([]string(nil), wf.Steps...)
	return apply(projectRoot, adapter, []packages.Package{scoped}, &WorkflowSequence{Name: wf.Name, Steps: wf.Steps})
}

func apply(projectRoot string, adapter provider.Provider, pkgs []packages.Package, wf *WorkflowSequence) error {
	prev, err := loadState(projectRoot, adapter.Name)
	if err != nil {
		return err
	}

	desired, err := desiredSkillDirs(adapter, pkgs)
	if err != nil {
		return err
	}

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

	if err := writeSummary(filepath.Join(projectRoot, adapter.ContextFile), pkgs, wf); err != nil {
		return fmt.Errorf("update %s: %w", adapter.ContextFile, err)
	}

	return saveState(projectRoot, adapter.Name, state{Schema: currentStateSchema, SkillDirs: written})
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

	desired, err := desiredSkillDirs(adapter, pkgs)
	if err != nil {
		return false, err
	}
	desiredDirs := make([]string, 0, len(desired))
	for rel := range desired {
		desiredDirs = append(desiredDirs, rel)
	}
	sort.Strings(desiredDirs)

	prevDirs := append([]string(nil), prev.SkillDirs...)
	sort.Strings(prevDirs)

	return !equalStrings(desiredDirs, prevDirs), nil
}

// NeedsApprovalForWorkflow is NeedsApproval scoped to a single workflow's
// steps, matching what ApplyWorkflow would actually stage.
func NeedsApprovalForWorkflow(projectRoot string, adapter provider.Provider, pkg packages.Package, wf packages.Workflow) (bool, error) {
	scoped := pkg
	scoped.Skills = append([]string(nil), wf.Steps...)
	return NeedsApproval(projectRoot, adapter, []packages.Package{scoped})
}

// desiredSkillDirs computes each enabled skill's staged directory name by
// joining package name and skill name with "-". Neither is restricted
// enough to make that join unambiguous on its own (manifest names can
// contain "-", and hyphenated skill names like "commit-messages" are
// normal): package "foo-bar" skill "x" and package "foo" skill "bar-x"
// both produce "foo-bar-x". Rather than change the naming scheme (which
// would make every staged directory harder to read, for a collision that
// almost never happens), a genuine collision is detected and reported as
// an error instead of one entry silently overwriting the other in the map.
func desiredSkillDirs(adapter provider.Provider, pkgs []packages.Package) (map[string]string, error) {
	desired := map[string]string{} // relative skill dir -> absolute source dir
	for _, pkg := range pkgs {
		for _, skill := range pkg.Skills {
			rel := filepath.Join(adapter.SkillsDir, pkg.Manifest.Name+"-"+skill)
			src := filepath.Join(pkg.Path, "skills", skill)
			if existing, ok := desired[rel]; ok && existing != src {
				return nil, fmt.Errorf("skill directory name %q is claimed by both %s and %s - rename one of these skills or packages to disambiguate", rel, existing, src)
			}
			desired[rel] = src
		}
	}
	return desired, nil
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
	// An absent schema field and an explicit `"schema": 0` both decode to
	// zero. Probe the field as a pointer so only the absent legacy field
	// defaults to schema 1; explicit zero remains unsupported.
	var probe struct {
		Schema *int `json:"schema"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return state{}, fmt.Errorf("parse %s: %w", statePath(projectRoot, providerName), err)
	}
	if probe.Schema == nil {
		s.Schema = currentStateSchema
	}
	if s.Schema != currentStateSchema {
		return state{}, fmt.Errorf("%s declares schema %d, but this build only understands schema %d", statePath(projectRoot, providerName), s.Schema, currentStateSchema)
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
	return atomicfile.WriteFile(path, data, 0o644)
}

func writeSummary(path string, pkgs []packages.Package, wf *WorkflowSequence) error {
	block := renderSummaryBlock(pkgs, wf)

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	next := block
	if err == nil {
		next = replaceBlock(string(existing), block)
	}

	return atomicfile.WriteFile(path, []byte(next), 0o644)
}

func renderSummaryBlock(pkgs []packages.Package, wf *WorkflowSequence) string {
	var b strings.Builder
	b.WriteString(beginMarker + "\n")
	b.WriteString("Lineage-managed section. Do not edit by hand; it is regenerated on every `lineage run`.\n\n")

	if wf != nil {
		fmt.Fprintf(&b, "Active workflow: %s\n", wf.Name)
		b.WriteString("Follow these skills in order:\n\n")
		for i, step := range wf.Steps {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, step)
		}
		b.WriteString("\n")
	}

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

	// Cap at 0o755 rather than copying the source file's mode verbatim: a
	// package could otherwise ship a skill file with 0o666/0o777
	// permissions and have that exact mode replicated into the receiver's
	// project, potentially leaving world-writable files behind on a
	// multi-user machine. Masking with 0o755 keeps any owner/group/other
	// read or execute bits the source had, but can never grant group or
	// other write access regardless of what the source declared. Chmod'd
	// explicitly after creation rather than only passed to OpenFile,
	// since OpenFile's mode argument is itself subject to the process's
	// umask - relying on that alone would make the cap only as strong as
	// whatever umask happens to be in effect on the receiver's machine,
	// rather than a guarantee this codebase actually makes.
	perm := info.Mode().Perm() & 0o755
	out, err := atomicfile.Create(dest, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Commit()
}
