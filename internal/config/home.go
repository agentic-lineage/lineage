package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const HomeEnv = "LINEAGE_HOME"

func HomeDir() (string, error) {
	if value := os.Getenv(HomeEnv); value != "" {
		return filepath.Abs(value)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".lineage"), nil
}

func UserPackagesDir(home string) string {
	return filepath.Join(home, "user", "packages")
}

func WorkspacesDir(home string) string {
	return filepath.Join(home, "workspaces")
}

func WorkspacePackagesDir(home, name string) string {
	return filepath.Join(WorkspacesDir(home), name, "packages")
}

// workspaceNamePattern is the safe charset for a workspace name: it has to
// survive being used as a single directory name under WorkspacesDir on
// every supported platform, so no separators, no drive letters, no
// leading dot.
var workspaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateWorkspaceName rejects names that would not stay inside
// WorkspacesDir once joined, or that are not usable as a directory name.
// A workspace name is a plain identifier, not a path: callers pass it
// straight to WorkspacePackagesDir, where "../.." would otherwise escape
// the Lineage home entirely.
func ValidateWorkspaceName(name string) error {
	if name == "" {
		return fmt.Errorf("workspace name must not be empty")
	}
	if !workspaceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid workspace name %q: use letters, digits, dot, dash, or underscore, starting with a letter or digit", name)
	}
	return nil
}

func ShimsDir(home string) string {
	return filepath.Join(home, "bin")
}

// ObjectsDir is where internal/snapshot stores individual content-addressed
// file objects, kept separate from SnapshotsDir so a snapshot manifest and
// the objects it references live in distinct namespaces on disk.
func ObjectsDir(home string) string {
	return filepath.Join(home, "objects")
}

// SnapshotsDir is where internal/snapshot stores content-addressed snapshot
// manifests, addressed the same way as ObjectsDir's file objects but never
// commingled with them.
func SnapshotsDir(home string) string {
	return filepath.Join(home, "snapshots")
}
