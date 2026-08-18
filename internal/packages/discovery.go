package packages

import (
	"fmt"
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
	pkg.Skills = discoverSkillNames(filepath.Join(dir, "skills"))
	pkg.Workflows = discoverNamesWithFile(filepath.Join(dir, "workflows"), "WORKFLOW.md")
	pkg.Agents = discoverEntryNames(filepath.Join(dir, "agents"))
	pkg.Policies = discoverEntryNames(filepath.Join(dir, "policies"))
	return pkg, nil
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
