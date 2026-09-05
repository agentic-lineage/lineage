//go:build !windows

package materialize

import (
	"os"
	"testing"
)

// assertNoLooseWriteBits checks that copyFile's 0o755 cap actually stripped
// the group/other write bits a loose source file (0o777) would otherwise
// carry over - a check only POSIX permission bits can make.
func assertNoLooseWriteBits(t *testing.T, mode os.FileMode) {
	t.Helper()
	if mode.Perm()&0o022 != 0 {
		t.Errorf("staged file mode = %v, want no group/other write bit (source was 0o777, umask forced to 0 so the OS can't mask this for us)", mode.Perm())
	}
}
