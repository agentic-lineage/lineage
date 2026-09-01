//go:build windows

package atomicfile

import (
	"os"
	"testing"
)

// assertWriteFilePermissions checks the one bit Windows actually models:
// Chmod's write bits map to the read-only file attribute, so a mode with
// any write bit set (like 0o755) must produce a non-read-only file. Windows
// has no concept of the remaining Unix permission bits, so it won't
// reproduce 0o755 verbatim - it reports something like -rw-rw-rw-.
func assertWriteFilePermissions(t *testing.T, mode os.FileMode) {
	t.Helper()
	if mode.Perm()&0o200 == 0 {
		t.Errorf("mode = %v, want owner-writable (requested mode had a write bit set, so the file must not end up read-only)", mode.Perm())
	}
}
