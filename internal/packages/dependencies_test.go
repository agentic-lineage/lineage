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
