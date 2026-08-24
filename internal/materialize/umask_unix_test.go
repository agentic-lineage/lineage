//go:build !windows

package materialize

import "syscall"

// withUmask0 runs fn with the process umask temporarily set to 0, so a
// permission-related test isn't at the mercy of whatever umask the test
// happens to run under (a standard 0o022 umask would strip the same bits
// os.OpenFile's mode argument would ask for anyway, hiding the difference
// between "the code asked for a safe mode" and "the umask saved us").
// Windows has no umask concept in this sense, so its variant is a no-op.
func withUmask0(fn func()) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	fn()
}
