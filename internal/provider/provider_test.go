package provider

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
)

func TestIsShimPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path prefix semantics differ on Windows")
	}

	home := t.TempDir()
	shimPath := filepath.Join(home, "bin", "claude")
	if !IsShimPath(shimPath, home) {
		t.Fatalf("IsShimPath(%q) = false, want true", shimPath)
	}
	realPath := filepath.Join(home, "real", "claude")
	if IsShimPath(realPath, home) {
		t.Fatalf("IsShimPath(%q) = true, want false", realPath)
	}
}

func TestResolveWithConfiguredBinaryForEveryProvider(t *testing.T) {
	for _, adapter := range Known() {
		t.Run(adapter.Name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), adapter.Name)
			project := config.ProjectConfig{
				Providers: map[string]config.Provider{
					adapter.Name: {
						Binary: binary,
						Args:   []string{"configured-arg"},
					},
				},
			}

			plan, err := Resolve(
				adapter.Name, t.TempDir(), project, []string{"launch-arg"},
			)

			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if plan.Name != adapter.Name {
				t.Errorf("Name = %q, want %q", plan.Name, adapter.Name)
			}

			if plan.Binary != binary {
				t.Errorf("Binary = %q, want %q", plan.Binary, binary)
			}

			wantArgs := []string{"configured-arg", "launch-arg"}
			if !reflect.DeepEqual(plan.Args, wantArgs) {
				t.Errorf("Args = %#v, want %#v", plan.Args, wantArgs)
			}

			if !containsString(plan.Env, "LINEAGE_PROVIDER="+adapter.Name) {
				t.Errorf("Env does not contain LINEAGE_PROVIDER=%s", adapter.Name)
			}

			if !containsString(plan.Env, "LINEAGE_ACTIVE=1") {
				t.Error("Env does not contain LINEAGE_ACTIVE=1")
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

func TestCandidateBinariesFindsMultipleAndSkipsShim(t *testing.T) {
	home := t.TempDir()
	dirA := t.TempDir()
	dirB := t.TempDir()
	shimDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeExecutable(t, filepath.Join(dirA, "claude"))
	writeExecutable(t, filepath.Join(dirB, "claude"))
	writeExecutable(t, filepath.Join(shimDir, "claude"))

	t.Setenv("PATH", strings.Join([]string{dirA, shimDir, dirB}, string(os.PathListSeparator)))

	candidates := CandidateBinaries("claude", home)
	if len(candidates) != 2 {
		t.Fatalf("CandidateBinaries() = %#v, want 2 real candidates (shim excluded)", candidates)
	}
	if candidates[0] != filepath.Join(dirA, "claude") || candidates[1] != filepath.Join(dirB, "claude") {
		t.Fatalf("CandidateBinaries() = %#v, want PATH order [%s, %s]", candidates, dirA, dirB)
	}
}

// TestCandidateBinariesSkipsShimContentOutsideShimsDir covers a regression
// where only a shim's *directory* was excluded (config.ShimsDir(home)),
// not its content. A shim installed under a different LINEAGE_HOME earlier
// - or a stray copy of the shim script anywhere else on PATH - would
// otherwise pass directory-based exclusion and be treated as a real
// binary; launching it would exec straight back into `lineage run`,
// resolving the same wrong candidate again with no self-detection: an
// unbounded fork loop.
func TestCandidateBinariesSkipsShimContentOutsideShimsDir(t *testing.T) {
	home := t.TempDir()
	realDir := t.TempDir()
	strayDir := t.TempDir() // NOT config.ShimsDir(home) - simulates a shim installed under a different LINEAGE_HOME, or copied elsewhere

	writeExecutable(t, filepath.Join(realDir, "claude"))
	strayShim := filepath.Join(strayDir, "claude")
	if err := os.WriteFile(strayShim, []byte("#!/bin/sh\nexec \"/some/other/lineage\" run claude \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", strings.Join([]string{strayDir, realDir}, string(os.PathListSeparator)))

	candidates := CandidateBinaries("claude", home)
	if len(candidates) != 1 || candidates[0] != filepath.Join(realDir, "claude") {
		t.Fatalf("CandidateBinaries() = %#v, want only the real binary in %s (the shim-content stray copy in %s excluded)", candidates, realDir, strayDir)
	}
}

func TestCandidateBinariesEmptyWhenNoneFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())

	candidates := CandidateBinaries("does-not-exist", home)
	if len(candidates) != 0 {
		t.Fatalf("CandidateBinaries() = %#v, want none", candidates)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateExtensionsPOSIXIsExactName(t *testing.T) {
	exts := candidateExtensions("linux")
	if len(exts) != 1 || exts[0] != "" {
		t.Fatalf("candidateExtensions(linux) = %#v, want a single empty extension", exts)
	}
}

func TestCandidateExtensionsWindowsUsesDefaultPathext(t *testing.T) {
	t.Setenv("PATHEXT", "")
	exts := candidateExtensions("windows")
	want := []string{".COM", ".EXE", ".BAT", ".CMD"}
	if len(exts) != len(want) {
		t.Fatalf("candidateExtensions(windows) = %#v, want %#v", exts, want)
	}
	for i := range want {
		if exts[i] != want[i] {
			t.Fatalf("candidateExtensions(windows) = %#v, want %#v", exts, want)
		}
	}
}

func TestCandidateExtensionsWindowsHonorsPathextEnv(t *testing.T) {
	t.Setenv("PATHEXT", ".EXE;.CMD")
	exts := candidateExtensions("windows")
	if len(exts) != 2 || exts[0] != ".EXE" || exts[1] != ".CMD" {
		t.Fatalf("candidateExtensions(windows) = %#v, want [.EXE .CMD] from PATHEXT", exts)
	}
}

func TestCandidateBinariesForWindowsFindsCmdShimTarget(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	// Real Windows filesystems (NTFS) are case-insensitive, so PATHEXT's
	// conventional uppercase wouldn't matter there. This test's fixture
	// file and PATHEXT value are kept in matching case deliberately: a
	// case-sensitive host filesystem (e.g. this repo's Linux CI) must not
	// make an otherwise-correct Windows-branch test flaky depending on
	// what the test happens to be running on.
	t.Setenv("PATHEXT", ".exe;.cmd")

	// On Windows, a permission bit isn't what makes a file "runnable" as a
	// bare command - existing under a PATHEXT-listed extension is. Deny
	// write/exec-looking perms to prove the Windows branch doesn't gate on
	// the POSIX executable bit the way the POSIX branch does.
	if err := os.WriteFile(filepath.Join(dir, "claude.cmd"), []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	candidates := candidateBinariesFor("claude", home, "windows")
	want := filepath.Join(dir, "claude.cmd")
	if len(candidates) != 1 || candidates[0] != want {
		t.Fatalf("candidateBinariesFor(windows) = %#v, want [%s]", candidates, want)
	}
}

func TestCandidateBinariesForWindowsSkipsShimDirRegardlessOfExtension(t *testing.T) {
	home := t.TempDir()
	shimDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shimDir, "claude.cmd"), []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATHEXT", ".exe;.cmd")
	t.Setenv("PATH", shimDir)

	candidates := candidateBinariesFor("claude", home, "windows")
	if len(candidates) != 0 {
		t.Fatalf("candidateBinariesFor(windows) = %#v, want the shim directory excluded", candidates)
	}
}
