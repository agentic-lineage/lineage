package packages

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Package struct {
	Path      string
	Manifest  Manifest
	Skills    []string
	Workflows []string
	Agents    []string
	Policies  []string
	// Digest is a sha256 content digest over the manifest and every file in
	// the package's standard content directories, in deterministic order.
	// Identical package content always produces the same digest.
	Digest string
}

func Discover(dir string) (Package, error) {
	manifest, err := LoadManifest(dir)
	if err != nil {
		return Package{}, err
	}

	pkg := Package{
		Path:     dir,
		Manifest: manifest,
	}

	if err := validateEntrypoint(dir, "claude", manifest.Entrypoints.Claude); err != nil {
		return Package{}, fmt.Errorf("%s: %w", dir, err)
	}
	if err := validateEntrypoint(dir, "codex", manifest.Entrypoints.Codex); err != nil {
		return Package{}, fmt.Errorf("%s: %w", dir, err)
	}

	workflows, err := applyExportAuthority("workflow", manifest.Exports.Workflows, discoverNamesWithFile(filepath.Join(dir, "workflows"), "WORKFLOW.md"))
	if err != nil {
		return Package{}, fmt.Errorf("%s: %w", dir, err)
	}
	pkg.Workflows = workflows

	agents, err := applyExportAuthority("agent", manifest.Exports.Agents, discoverEntryNames(filepath.Join(dir, "agents")))
	if err != nil {
		return Package{}, fmt.Errorf("%s: %w", dir, err)
	}
	pkg.Agents = agents

	pkg.Skills = discoverSkillNames(filepath.Join(dir, "skills"))
	pkg.Policies = discoverEntryNames(filepath.Join(dir, "policies"))

	digest, err := ComputeDigest(dir)
	if err != nil {
		return Package{}, fmt.Errorf("%s: %w", dir, err)
	}
	pkg.Digest = digest

	return pkg, nil
}

// applyExportAuthority implements "manifest is authoritative, filesystem is
// payload": when the manifest explicitly declares a non-empty export list,
// every declared name must actually be discoverable on disk (a missing one
// is a validation error), and only declared names count as part of the
// package's exported set — anything discovered but not declared is allowed
// to exist locally but is excluded from the result. An empty/omitted
// declaration means "not yet declared", not "declares nothing": it falls
// back to whatever's discovered on disk, matching pre-schema behavior so
// existing packages that never declared exports keep working unchanged.
func applyExportAuthority(kind string, declared, discovered []string) ([]string, error) {
	if len(declared) == 0 {
		return discovered, nil
	}

	if missing := missingDeclared(declared, discovered); len(missing) > 0 {
		return nil, fmt.Errorf("manifest declares %s %q but it was not found", kind, missing[0])
	}

	result := append([]string(nil), declared...)
	sort.Strings(result)
	return result, nil
}

// validateEntrypoint checks a manifest-declared entrypoint path (untrusted,
// package-controlled input) stays within the package directory before
// anything downstream ever reads it. An unset entrypoint is fine — the
// field is optional.
func validateEntrypoint(dir, provider, entrypoint string) error {
	if entrypoint == "" {
		return nil
	}
	if _, err := SafeJoin(dir, entrypoint); err != nil {
		return fmt.Errorf("entrypoints.%s: %w", provider, err)
	}
	return nil
}

// ComputeDigest returns a stable "sha256:<hex>" content digest over a
// package's manifest and every file in its standard content directories
// (skills, workflows, agents, policies, references, adapters), hashed in
// deterministic path order. Two packages with byte-identical content always
// produce the same digest; any content change changes it. A symlink
// anywhere in those directories is rejected rather than followed, since
// package content is untrusted and a symlink could otherwise pull
// arbitrary file content from outside the package into the digest.
func ComputeDigest(dir string) (string, error) {
	h := sha256.New()

	manifestData, err := os.ReadFile(filepath.Join(dir, ManifestFileName))
	if err != nil {
		return "", fmt.Errorf("read manifest for digest: %w", err)
	}
	h.Write(manifestData)

	var files []string
	for _, sub := range []string{"skills", "workflows", "agents", "policies", "references", "adapters"} {
		root := filepath.Join(dir, sub)
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				rel, relErr := filepath.Rel(dir, path)
				if relErr != nil {
					rel = path
				}
				return fmt.Errorf("refusing to include symlink %s in digest", rel)
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				return nil
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if walkErr != nil {
			return "", walkErr
		}
	}
	sort.Strings(files)

	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("read %s for digest: %w", rel, err)
		}
		h.Write([]byte(rel))
		h.Write(data)
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// InitPackage scaffolds a package directory: a lineage.yaml manifest plus
// the standard skills/workflows/agents/policies/references/adapters
// subdirectories. It is safe to call more than once against the same
// directory. If a manifest already exists there, InitPackage leaves it
// untouched rather than resetting it to defaults, so re-running `lineage
// package init` never silently discards a maintainer's version bump,
// description edit, or other manifest changes.
func InitPackage(dir, name string) error {
	manifestPath := filepath.Join(dir, ManifestFileName)
	switch _, err := os.Stat(manifestPath); {
	case err == nil:
		// Manifest already present; nothing to do here.
	case os.IsNotExist(err):
		if err := SaveManifest(dir, DefaultManifest(name)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("check existing manifest %s: %w", manifestPath, err)
	}

	for _, child := range []string{"skills", "workflows", "agents", "policies", "references", "adapters"} {
		if err := os.MkdirAll(filepath.Join(dir, child), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", child, err)
		}
	}
	return nil
}

func ResolveEnabled(home string, workspace string, projectRoot string, enabled []string) ([]Package, error) {
	var resolved []Package
	for _, ref := range enabled {
		path, err := ResolveReference(home, workspace, projectRoot, ref)
		if err != nil {
			return nil, err
		}
		pkg, err := Discover(path)
		if err != nil {
			return nil, fmt.Errorf("discover package %q: %w", ref, err)
		}
		resolved = append(resolved, pkg)
	}
	return resolved, nil
}

func ResolveReference(home string, workspace string, projectRoot string, ref string) (string, error) {
	candidates := []string{}
	if filepath.IsAbs(ref) {
		candidates = append(candidates, ref)
	} else if strings.HasPrefix(ref, ".") {
		candidates = append(candidates, filepath.Join(projectRoot, ref))
	} else {
		candidates = append(candidates,
			filepath.Join(projectRoot, ".lineage", "packages", ref),
			filepath.Join(home, "user", "packages", ref),
		)
		if workspace != "" {
			candidates = append(candidates, filepath.Join(home, "workspaces", workspace, "packages", ref))
		}
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, ManifestFileName)); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("package %q not found in project, user, or workspace packages", ref)
}

func discoverSkillNames(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}

	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func discoverNamesWithFile(root, fileName string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}

	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), fileName)); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func discoverEntryNames(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}

	names := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}
