package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lineage-dev/lineage/internal/config"
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

func TestResolveWithConfiguredBinary(t *testing.T) {
	project := config.ProjectConfig{
		Providers: map[string]config.Provider{
			"codex": {Binary: "/bin/echo"},
		},
	}
	plan, err := Resolve("codex", t.TempDir(), project, []string{"hello"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if plan.Binary != "/bin/echo" {
		t.Fatalf("Binary = %q", plan.Binary)
	}
	if len(plan.Args) != 1 || plan.Args[0] != "hello" {
		t.Fatalf("Args = %#v", plan.Args)
	}
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
	t.Setenv("PATHEXT", ".EXE;.CMD")

	// On Windows, a permission bit isn't what makes a file "runnable" as a
	// bare command - existing under a PATHEXT-listed extension is. Deny
	// write/exec-looking perms to prove the Windows branch doesn't gate on
	// the POSIX executable bit the way the POSIX branch does.
	if err := os.WriteFile(filepath.Join(dir, "claude.cmd"), []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	candidates := candidateBinariesFor("claude", home, "windows")
	// Compared case-insensitively: PATHEXT is conventionally uppercase
	// (.CMD) while the file on disk is lowercase (claude.cmd), and both
	// NTFS and the case-insensitive filesystem this test may run on
	// resolve that the same way real Windows would - the constructed
	// candidate path is valid even when its case doesn't match the
	// on-disk filename byte-for-byte.
	if len(candidates) != 1 || !strings.EqualFold(candidates[0], filepath.Join(dir, "claude.cmd")) {
		t.Fatalf("candidateBinariesFor(windows) = %#v, want [%s] (case-insensitive)", candidates, filepath.Join(dir, "claude.cmd"))
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
	t.Setenv("PATHEXT", ".EXE;.CMD")
	t.Setenv("PATH", shimDir)

	candidates := candidateBinariesFor("claude", home, "windows")
	if len(candidates) != 0 {
		t.Fatalf("candidateBinariesFor(windows) = %#v, want the shim directory excluded", candidates)
	}
}
