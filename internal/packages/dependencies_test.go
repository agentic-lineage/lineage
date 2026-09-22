package packages

import "testing"

func TestValidateDependenciesSatisfiedByOwnSkill(t *testing.T) {
	pkg := Package{
		Manifest: Manifest{Name: "self-sufficient", Requires: Requires{Skills: []string{"review"}}},
		Skills:   []string{"review"},
	}
	if err := ValidateDependencies([]Package{pkg}); err != nil {
		t.Fatalf("ValidateDependencies() error = %v", err)
	}
}

func TestValidateDependenciesSatisfiedByAnotherPackage(t *testing.T) {
	dependent := Package{
		Manifest: Manifest{Name: "dependent", Requires: Requires{Skills: []string{"security-basics"}}},
	}
	provider := Package{
		Manifest: Manifest{Name: "provider"},
		Skills:   []string{"security-basics"},
	}
	if err := ValidateDependencies([]Package{dependent, provider}); err != nil {
		t.Fatalf("ValidateDependencies() error = %v", err)
	}
}

func TestValidateDependenciesMissingSkillFails(t *testing.T) {
	dependent := Package{
		Manifest: Manifest{Name: "dependent", Requires: Requires{Skills: []string{"nonexistent"}}},
	}
	err := ValidateDependencies([]Package{dependent})
	if err == nil {
		t.Fatal("ValidateDependencies() error = nil, want error for missing required skill")
	}
}

func TestValidateDependenciesDuplicateExportFails(t *testing.T) {
	foo := Package{
		Manifest: Manifest{Name: "foo"},
		Skills:   []string{"research"},
	}
	bar := Package{
		Manifest: Manifest{Name: "bar"},
		Skills:   []string{"research"},
	}
	err := ValidateDependencies([]Package{foo, bar})
	if err == nil {
		t.Fatal("ValidateDependencies() error = nil, want error for duplicate skill export")
	}
	want := `cannot enable package set: skill "research" is exported by both "bar" and "foo"`
	if err.Error() != want {
		t.Fatalf("ValidateDependencies() error = %q, want %q", err.Error(), want)
	}
}

func TestValidateDependenciesDuplicateExportIndependentOfOrder(t *testing.T) {
	foo := Package{Manifest: Manifest{Name: "foo"}, Skills: []string{"research"}}
	bar := Package{Manifest: Manifest{Name: "bar"}, Skills: []string{"research"}}
	first := ValidateDependencies([]Package{foo, bar})
	second := ValidateDependencies([]Package{bar, foo})
	if first == nil || second == nil {
		t.Fatal("ValidateDependencies() error = nil, want error for duplicate skill export in both orders")
	}
	if first.Error() != second.Error() {
		t.Fatalf("ValidateDependencies() error differs by iteration order: %q vs %q", first.Error(), second.Error())
	}
}

func TestValidateDependenciesTripleExportNamesAllProviders(t *testing.T) {
	pkgs := []Package{
		{Manifest: Manifest{Name: "zeta"}, Skills: []string{"research"}},
		{Manifest: Manifest{Name: "alpha"}, Skills: []string{"research"}},
		{Manifest: Manifest{Name: "mid"}, Skills: []string{"research"}},
	}
	err := ValidateDependencies(pkgs)
	if err == nil {
		t.Fatal("ValidateDependencies() error = nil, want error for triple skill export")
	}
	want := `cannot enable package set: skill "research" is exported by multiple packages: "alpha", "mid", "zeta"`
	if err.Error() != want {
		t.Fatalf("ValidateDependencies() error = %q, want %q", err.Error(), want)
	}
}

func TestValidateDependenciesSelfRequirementIsNotAConflict(t *testing.T) {
	pkg := Package{
		Manifest: Manifest{Name: "self-sufficient", Requires: Requires{Skills: []string{"review"}}},
		Skills:   []string{"review"},
	}
	if err := ValidateDependencies([]Package{pkg}); err != nil {
		t.Fatalf("ValidateDependencies() error = %v", err)
	}
}
