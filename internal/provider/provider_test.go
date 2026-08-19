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
