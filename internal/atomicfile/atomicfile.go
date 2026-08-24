// Package atomicfile writes files so a crash, kill, or disk-full error
// mid-write can never leave a corrupt or truncated file at the destination
// path - a reader always sees either the previous content or the complete
// new content, never a mix. It does this the standard way: write to a temp
// file in the same directory (so the final rename stays on one filesystem),
// then atomically rename that temp file over the destination.
//
// Nothing in this codebase should write a file that matters - staged
// package content, generated context files, project config, shims,
// imported/exported archives - with a plain os.WriteFile or os.Create.
package atomicfile

import (
	"os"
	"path/filepath"
)

// File is an in-progress atomic write. Call Write (directly, or via
// io.Copy) to stream content into it, then exactly one of Commit (to
// atomically publish it at the destination path) or Close (to discard it).
// Calling Close after a successful Commit is a safe no-op, so a deferred
// Close alongside an explicit Commit is the normal usage pattern - the
// defer only does anything if the function returns before Commit runs.
type File struct {
	f       *os.File
	tmpPath string
	path    string
	perm    os.FileMode
	closed  bool
}

// Create opens a new temp file in the same directory as path, ready to be
// written to and later committed there. The directory is created if it
// doesn't already exist.
func Create(path string, perm os.FileMode) (*File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, ".atomicfile-*")
	if err != nil {
		return nil, err
	}
	return &File{f: tmp, tmpPath: tmp.Name(), path: path, perm: perm}, nil
}

// Write implements io.Writer against the underlying temp file.
func (f *File) Write(p []byte) (int, error) {
	return f.f.Write(p)
}

// Commit flushes and closes the temp file, sets its final permissions, and
// atomically renames it into place at the destination path. The File must
// not be used after Commit returns, successfully or not.
func (f *File) Commit() error {
	defer func() { f.closed = true }()

	if err := f.f.Sync(); err != nil {
		f.f.Close()
		os.Remove(f.tmpPath)
		return err
	}
	if err := f.f.Close(); err != nil {
		os.Remove(f.tmpPath)
		return err
	}
	if err := os.Chmod(f.tmpPath, f.perm); err != nil {
		os.Remove(f.tmpPath)
		return err
	}
	if err := os.Rename(f.tmpPath, f.path); err != nil {
		os.Remove(f.tmpPath)
		return err
	}
	return nil
}

// Close discards the write: closes and removes the temp file without ever
// touching the destination path. A no-op if Commit already ran (whether it
// succeeded or failed - both leave nothing at tmpPath to clean up), so it's
// safe to defer unconditionally alongside an explicit Commit call.
func (f *File) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	f.f.Close()
	return os.Remove(f.tmpPath)
}

// WriteFile atomically writes data to path: the equivalent of os.WriteFile,
// but via Create/Write/Commit so a failure or interruption partway through
// never leaves a partially-written file at path.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	f, err := Create(path, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Commit()
}
