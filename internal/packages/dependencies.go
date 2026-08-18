package packages

import "fmt"

// ValidateDependencies checks that every package's Requires.Skills is
// satisfied somewhere across pkgs — its own skills, or another package's.
// It's meant to run against the full set of packages that will be enabled
// together (not one package in isolation), since a required skill can
// legitimately be provided by a different enabled package.
func ValidateDependencies(pkgs []Package) error {
	available := map[string]bool{}
	for _, pkg := range pkgs {
		for _, skill := range pkg.Skills {
			available[skill] = true
		}
	}

	for _, pkg := range pkgs {
		for _, required := range pkg.Manifest.Requires.Skills {
			if !available[required] {
				return fmt.Errorf("package %q requires skill %q, which is not provided by any enabled package", pkg.Manifest.Name, required)
			}
		}
	}
	return nil
}
