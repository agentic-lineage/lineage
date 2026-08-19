package shim

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lineage-dev/lineage/internal/provider"
)

// Install writes a shim for every registered provider (internal/provider's
// registry is the single source of truth for provider names - adding a
// provider there is enough to get a shim for it here too) into home's shim
// directory, in the format the current OS actually runs.
func Install(home, lineageBinary string) error {
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create shim directory: %w", err)
	}

	for _, p := range provider.Known() {
		if err := writeShim(binDir, lineageBinary, p.Name, runtime.GOOS); err != nil {
			return err
		}
	}
	return nil
}

// FileName returns the shim's filename for a provider on goos. Windows
// resolves bare commands by extension (PATHEXT), so the shim needs a
// recognized one (.cmd); POSIX shells execute by the executable bit, no
// extension needed. Exported so internal/provider's shim/binary detection
// constructs the same name rather than duplicating the convention.
func FileName(providerName, goos string) string {
	if goos == "windows" {
		return providerName + ".cmd"
	}
	return providerName
}

func writeShim(binDir, lineageBinary, providerName, goos string) error {
	path := filepath.Join(binDir, FileName(providerName, goos))
	content, perm := shimContent(lineageBinary, providerName, goos)
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return fmt.Errorf("write %s shim: %w", providerName, err)
	}
	return nil
}

func shimContent(lineageBinary, providerName, goos string) (content string, perm os.FileMode) {
	if goos == "windows" {
		// %* forwards every argument verbatim, same intent as POSIX "$@".
		// Batch files conventionally use CRLF line endings.
		return fmt.Sprintf("@echo off\r\n\"%s\" run %s %%*\r\n", lineageBinary, providerName), 0o644
	}
	return fmt.Sprintf("#!/bin/sh\nexec %q run %s \"$@\"\n", lineageBinary, providerName), 0o755
}
