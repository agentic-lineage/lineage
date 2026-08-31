//go:build windows

package materialize

import (
	"os"
	"testing"
)

func assertNoLooseWriteBits(t *testing.T, mode os.FileMode) {
	t.Helper()
	t.Fatal("assertNoLooseWriteBits should only be called from POSIX tests")
}
