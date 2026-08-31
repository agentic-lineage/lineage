package shim

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileNamePOSIXHasNoExtension(t *testing.T) {
	if got := FileName("claude", "linux"); got != "claude" {
		t.Fatalf("FileName(linux) = %q, want %q", got, "claude")
	}
	if got := FileName("claude", "darwin"); got != "claude" {
		t.Fatalf("FileName(darwin) = %q, want %q", got, "claude")
	}
}

func TestFileNameWindowsHasCmdExtension(t *testing.T) {
	if got := FileName("claude", "windows"); got != "claude.cmd" {
		t.Fatalf("FileName(windows) = %q, want %q", got, "claude.cmd")
	}
}

func TestInstallWritesShimsForLaunchableProviders(t *testing.T) {
	home := t.TempDir()
	if err := Install(home, "/usr/local/bin/lineage"); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	for _, name := range []string{"claude", "codex", "aider"} {
		path := filepath.Join(home, "bin", FileName(name, runtime.GOOS))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected shim for %s at %s: %v", name, path, err)
		}
	}
}

func TestShimContentPOSIXIsExecutableShellScript(t *testing.T) {
	content, perm := shimContent("/usr/local/bin/lineage", "claude", "linux")
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Fatalf("content = %q, want a POSIX shebang", content)
	}
	if !strings.Contains(content, `run claude "$@"`) {
		t.Fatalf("content = %q, want it to forward args and name the provider", content)
	}
	if perm&0o111 == 0 {
		t.Fatalf("perm = %v, want the executable bit set", perm)
	}
}

func TestShimContentWindowsIsBatchFile(t *testing.T) {
	content, _ := shimContent(`C:\Program Files\lineage\lineage.exe`, "claude", "windows")
	if !strings.HasPrefix(content, "@echo off\r\n") {
		t.Fatalf("content = %q, want a batch file header", content)
	}
	if !strings.Contains(content, "run claude %*") {
		t.Fatalf("content = %q, want it to forward args and name the provider", content)
	}
	// The path must be quoted literally, not Go-%q-escaped - a
	// backslash-doubled path is not the same path to cmd.exe.
	if strings.Contains(content, `\\`) {
		t.Fatalf("content = %q, want the Windows path unescaped, not doubled", content)
	}
}
