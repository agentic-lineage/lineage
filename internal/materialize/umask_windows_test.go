//go:build windows

package materialize

// withUmask0 is a no-op on Windows, which has no POSIX umask concept -
// os.OpenFile's mode argument isn't masked the same way there.
func withUmask0(fn func()) {
	fn()
}
