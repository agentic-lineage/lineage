package packages

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ValidateDependencies checks that every package's Requires.Skills is
// satisfied somewhere across pkgs — its own skills, or another package's —
// and that no skill name is exported by more than one enabled package.
// It's meant to run against the full set of packages that will be enabled
// together (not one package in isolation), since a required skill can
// legitimately be provided by a different enabled package.
//
// A skill exported by two enabled packages is ambiguous: whichever
// definition wins later resolution order would silently satisfy the
// dependency, so validation fails instead of choosing (#246).
func ValidateDependencies(pkgs []Package) error {
	providers := map[string][]string{}
	for _, pkg := range pkgs {
		for _, skill := range pkg.Skills {
			if !slices.Contains(providers[skill], pkg.Manifest.Name) {
				providers[skill] = append(providers[skill], pkg.Manifest.Name)
			}
		}
	}

	for _, skill := range sortedKeys(providers) {
		pkgs := providers[skill]
		if len(pkgs) < 2 {
			continue
		}
		sort.Strings(pkgs)
		if len(pkgs) == 2 {
			return fmt.Errorf("cannot enable package set: skill %q is exported by both %q and %q", skill, pkgs[0], pkgs[1])
		}
		return fmt.Errorf("cannot enable package set: skill %q is exported by multiple packages: %s", skill, quotedJoin(pkgs))
	}

	for _, pkg := range pkgs {
		for _, required := range pkg.Manifest.Requires.Skills {
			if _, provided := providers[required]; !provided {
				return fmt.Errorf("package %q requires skill %q, which is not provided by any enabled package", pkg.Manifest.Name, required)
			}
		}
	}
	return nil
}

// sortedKeys returns providers' skill names in sorted order so the first
// conflict reported — and therefore the error message — is independent of
// package/discovery iteration order.
func sortedKeys(providers map[string][]string) []string {
	skills := make([]string, 0, len(providers))
	for skill := range providers {
		skills = append(skills, skill)
	}
	sort.Strings(skills)
	return skills
}

func quotedJoin(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, ", ")
}
