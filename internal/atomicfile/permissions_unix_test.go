//go:build !windows

package atomicfile

import (
	"os"
	"testing"
)

// assertWriteFilePermissions checks that WriteFile's requested permission
// bits round-trip exactly, which POSIX systems support in full.
func assertWriteFilePermissions(t *testing.T, mode os.FileMode) {
	t.Helper()
	if mode.Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", mode.Perm())
	}
}
