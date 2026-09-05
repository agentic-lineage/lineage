package packages

import (
	"fmt"
	"sort"
	"strings"
)

// ValidateDependencies checks two invariants over the set of packages that will
// be enabled together:
//
//  1. No two enabled packages export the same skill name. A skill name must
//     resolve to exactly one providing package, otherwise the launcher cannot
//     decide which implementation to wire into the run — and a silent,
//     order-dependent pick is worse than failing loudly. On conflict it
//     returns an error naming the skill and every package that exports it.
//
//  2. Every package's Requires.Skills is satisfied somewhere across pkgs — its
//     own skills, or another package's. It runs against the full enabled set
//     (not one package in isolation) because a required skill can legitimately
//     come from a different enabled package.
//
// Skill matching is by bare name only. Version-range resolution is not
// implemented: two packages exporting the same name at different versions still
// conflict, since there is no rule to pick between them.
//
// The same package may be supplied more than once when two enabled refs resolve
// to the same content; dedupePackages collapses those into a single provider so
// the duplicate is not mistaken for a self-conflict. When two genuinely
// different packages share a manifest name, the conflict error widens the
// identity past the bare name — to name@version, and to name@version with the
// resolved path when name@version still collides — so the operator can tell
// which copy to disable instead of reading `both "foo" and "foo"`.
func ValidateDependencies(pkgs []Package) error {
	pkgs = dedupePackages(pkgs)

	// skill -> packages that export it, in input order. A skill with more than
	// one provider is a conflict.
	providers := map[string][]Package{}
	for _, pkg := range pkgs {
		for _, skill := range pkg.Skills {
			providers[skill] = append(providers[skill], pkg)
		}
	}

	for _, pkg := range pkgs {
		// A package does not conflict with itself merely because it requires a
		// skill it also exports; that's the satisfied-by-own-skill case.
		for _, required := range pkg.Manifest.Requires.Skills {
			if _, ok := providers[required]; !ok {
				return fmt.Errorf("package %q requires skill %q, which is not provided by any enabled package", pkg.Manifest.Name, required)
			}
		}
	}

	// Report duplicate exports deterministically: sort by skill name so the
	// error is stable regardless of package/skill input order.
	var conflicts []string
	for skill, provs := range providers {
		if len(provs) < 2 {
			continue
		}
		identities := providerIdentities(provs)
		sort.Strings(identities)
		conflicts = append(conflicts, fmt.Sprintf("skill %q is exported by %s", skill, providerPhrase(identities)))
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return fmt.Errorf("cannot enable package set: %s", strings.Join(conflicts, "; "))
	}
	return nil
}

// dedupePackages collapses exact-same resolved packages so two enabled refs
// pointing at one package are counted as a single provider, not a
// self-conflict. Two packages are the same when their content digests match
// (the digest covers the manifest and every content file, so identical content
// always matches); when a digest is not available, the on-disk path is used
// instead. The first occurrence wins; later duplicates are dropped.
func dedupePackages(pkgs []Package) []Package {
	seen := make(map[string]struct{}, len(pkgs))
	out := make([]Package, 0, len(pkgs))
	for _, pkg := range pkgs {
		key := pkg.Digest
		if key == "" {
			key = pkg.Path
		}
		// Without a digest or path there is no identity signal to collapse on,
		// so two such packages are kept distinct rather than silently merged
		// under the empty-string key.
		if key == "" {
			out = append(out, pkg)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pkg)
	}
	return out
}

// providerIdentities renders the packages that export a skill as display
// strings detailed enough to tell them apart. When two enabled refs resolve to
// different package copies that share a manifest name, the bare name alone
// would make the conflict read `both "foo" and "foo"` and leave the operator
// unable to tell which copy to disable. The identity widens only as far as
// needed: the bare name when every provider's name is unique, then name@version
// when names collide but versions differ, then name@version with the resolved
// path appended for the residual collision.
func providerIdentities(provs []Package) []string {
	out := make([]string, len(provs))
	for i, p := range provs {
		out[i] = p.Manifest.Name
	}
	if allUnique(out) {
		return out
	}
	for i, p := range provs {
		out[i] = p.Manifest.Name + "@" + p.Manifest.Version
	}
	if allUnique(out) {
		return out
	}
	for i, p := range provs {
		out[i] = p.Manifest.Name + "@" + p.Manifest.Version + " (" + p.Path + ")"
	}
	return out
}

// allUnique reports whether xs contains no repeated string.
func allUnique(xs []string) bool {
	seen := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			return false
		}
		seen[x] = struct{}{}
	}
	return true
}

// providerPhrase renders a list of package names as a quoted phrase suitable
// for a conflict error: ["foo","bar"] -> `both "foo" and "bar"`, and
// ["a","b","c"] -> `"a", "b", and "c"`.
func providerPhrase(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	switch len(quoted) {
	case 0:
		return ""
	case 1:
		return quoted[0]
	case 2:
		return "both " + quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + ", and " + quoted[len(quoted)-1]
	}
}
