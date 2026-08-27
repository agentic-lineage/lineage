package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateWorkspaceName covers #171: `lineage init workspace
// ../../escaped` created a directory outside WorkspacesDir, because the
// name went straight into filepath.Join with no validation.
func TestValidateWorkspaceName(t *testing.T) {
	valid := []string{"team", "team-a", "team_a", "team.a", "Team1", "a"}
	for _, name := range valid {
		if err := ValidateWorkspaceName(name); err != nil {
			t.Errorf("ValidateWorkspaceName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "..", ".", "../../escaped", "a/b", `a\b`, ".hidden", "a b", "-leading"}
	for _, name := range invalid {
		if err := ValidateWorkspaceName(name); err == nil {
			t.Errorf("ValidateWorkspaceName(%q) = nil, want an error", name)
		}
	}
}

// TestValidateWorkspaceNameKeepsJoinInsideWorkspacesDir asserts the
// property the charset exists to protect, rather than only the charset.
func TestValidateWorkspaceNameKeepsJoinInsideWorkspacesDir(t *testing.T) {
	home := t.TempDir()
	root := WorkspacesDir(home)
	for _, name := range []string{"team", "team-a", "Team1.2_3"} {
		if err := ValidateWorkspaceName(name); err != nil {
			t.Fatalf("ValidateWorkspaceName(%q) = %v, want nil", name, err)
		}
		dir := WorkspacePackagesDir(home, name)
		if !strings.HasPrefix(filepath.Clean(dir), filepath.Clean(root)+string(filepath.Separator)) {
			t.Errorf("WorkspacePackagesDir(%q) = %q, want it under %q", name, dir, root)
		}
	}
}
