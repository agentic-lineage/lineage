package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello\n" {
		t.Errorf("content = %q, want %q", got, "hello\n")
	}
}

func TestWriteFileReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("content = %q, want %q", got, "new\n")
	}
}

func TestWriteFileLeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Fatalf("directory entries = %v, want only config.yaml (no leftover temp files)", entries)
	}
}

func TestCloseWithoutCommitLeavesDestinationUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := Create(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("never committed\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original\n" {
		t.Errorf("content = %q, want the original content untouched by an aborted write", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory entries = %v, want only the original file (no leftover temp file after Close)", entries)
	}
}

func TestCloseAfterCommitIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	f, err := Create(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() after a successful Commit() error = %v, want nil (no-op)", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello\n" {
		t.Errorf("content = %q, want %q", got, "hello\n")
	}
}

func TestWriteFileSetsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shim.sh")

	if err := WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
}
