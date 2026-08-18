package packages

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		return "", fmt.Errorf("package %q already exists at %s; remove it first or import with --as to use a different name", name, dest)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check destination %s: %w", dest, err)
	}

	if err := copyTree(tmp, dest); err != nil {
		os.RemoveAll(dest)
		return "", fmt.Errorf("install imported package: %w", err)
	}
	return name, nil
}

// extractArchive unpacks a tar.gz stream into destDir. Every entry's path
// is validated with SafeJoin before it's used — the archive is untrusted,
// package-controlled input, exactly like a manifest field. Only regular
// files and directories are accepted; anything else (symlinks, devices,
// hard links) is rejected outright rather than silently skipped.
func extractArchive(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
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
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.CopyN(out, r, size); err != nil {
		return err
	}
	return nil
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
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
