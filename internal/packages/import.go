package packages

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/agentic-lineage/lineage/internal/atomicfile"
)

// Import extracts an archive produced by Export into a new directory under
// destParent, named after the package's manifest (or asName, if given, to
// import under a different name than the one it was exported with).
//
// The archive is untrusted input the same way a manifest is: every entry
// path is checked with SafeJoin before anything is written, and the fully
// extracted content is run through Validate before it's kept — an archive
// that fails validation (a secret finding, a traversing entrypoint, a
// declared-but-missing export) is discarded, not imported. Import never
// overwrites an existing package directory; it fails rather than silently
// clobbering one.
//
// ErrAlreadyImported is returned by Import when destParent already has a
// package directory under the name Import resolved (asName, or the
// manifest's own name). Digest is the digest of the content Import was
// just asked to import, computed before it discovered the conflict - a
// caller that wants idempotent-reuse semantics (re-running the same
// import is a no-op, not a failure) can compare Digest against the
// existing directory's own digest instead of re-deriving either value.
type ErrAlreadyImported struct {
	Name   string
	Dest   string
	Digest string
}

func (e *ErrAlreadyImported) Error() string {
	return fmt.Sprintf("package %q already exists at %s; remove it first or import with --as to use a different name", e.Name, e.Dest)
}

// Import returns the name the package was imported under.
func Import(r io.Reader, destParent, asName string) (string, error) {
	tmp, err := os.MkdirTemp("", "lineage-import-*")
	if err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	if err := extractArchive(r, tmp); err != nil {
		return "", err
	}

	report, err := Validate(tmp)
	if err != nil {
		return "", err
	}
	if !report.Passed() {
		return "", fmt.Errorf("imported package failed validation, refusing to import (%d error(s))", len(report.Errors))
	}

	name := asName
	if name == "" {
		name = report.Manifest.Name
	}

	dest := filepath.Join(destParent, name)
	if _, err := os.Stat(dest); err == nil {
		return "", &ErrAlreadyImported{Name: name, Dest: dest, Digest: report.Digest}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check destination %s: %w", dest, err)
	}

	if err := copyTree(tmp, dest); err != nil {
		os.RemoveAll(dest)
		return "", fmt.Errorf("install imported package: %w", err)
	}
	return name, nil
}

// maxExtractedFileSize caps how large any single extracted file may be.
// Lineage packages are skills, workflows, and reference text - nothing
// legitimate needs anywhere near this much space in one file. A var, not a
// const, so tests can shrink it instead of needing real multi-megabyte
// fixtures to exercise the limit.
var maxExtractedFileSize int64 = 50 << 20 // 50MB

// maxExtractedTotalSize caps how much a whole archive may expand to once
// every entry is extracted. A tar header's declared size is trusted input
// the same way its path is: a small, highly-compressible archive can
// truthfully declare gigabytes of decompressed content, and io.CopyN will
// faithfully write exactly that many bytes if nothing stops it first -
// this is the classic decompression-bomb shape. Checked against the
// declared size before any of it is written, not after.
var maxExtractedTotalSize int64 = 200 << 20 // 200MB

// extractArchive unpacks a tar.gz stream into destDir. Every entry's path
// is validated with SafeJoin before it's used — the archive is untrusted,
// package-controlled input, exactly like a manifest field. Only regular
// files and directories are accepted; anything else (symlinks, devices,
// hard links) is rejected outright rather than silently skipped. Declared
// entry sizes are checked against maxExtractedFileSize/maxExtractedTotalSize
// before anything is written, so a maliciously small archive can't expand
// to fill the receiver's disk.
func extractArchive(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var totalSize int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if _, err := SafeJoin(destDir, hdr.Name); err != nil {
				return fmt.Errorf("archive entry %q: %w", hdr.Name, err)
			}
			continue
		case tar.TypeReg:
			// handled below
		default:
			return fmt.Errorf("archive entry %q has unsupported type; only regular files and directories are allowed", hdr.Name)
		}

		if hdr.Size < 0 {
			return fmt.Errorf("archive entry %q declares a negative size", hdr.Name)
		}
		if hdr.Size > maxExtractedFileSize {
			return fmt.Errorf("archive entry %q is %d bytes, over the %d byte per-file limit", hdr.Name, hdr.Size, maxExtractedFileSize)
		}
		totalSize += hdr.Size
		if totalSize > maxExtractedTotalSize {
			return fmt.Errorf("archive would extract to more than %d bytes total, refusing (possible decompression bomb)", maxExtractedTotalSize)
		}

		target, err := SafeJoin(destDir, hdr.Name)
		if err != nil {
			return fmt.Errorf("archive entry %q: %w", hdr.Name, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create directory for %q: %w", hdr.Name, err)
		}
		if err := writeArchiveFile(target, tr, hdr.Size); err != nil {
			return fmt.Errorf("extract %q: %w", hdr.Name, err)
		}
	}
	return nil
}

func writeArchiveFile(target string, r io.Reader, size int64) error {
	out, err := atomicfile.Create(target, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.CopyN(out, r, size); err != nil {
		return err
	}
	return out.Commit()
}

// copyTree recursively copies every file under src into dest, preserving
// the directory structure. Used to move a validated import out of its
// temporary staging directory into its final location.
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return atomicfile.WriteFile(target, data, 0o644)
	})
}
