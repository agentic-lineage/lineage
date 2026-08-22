package packages

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// deterministicTime is used for every tar entry's ModTime so that exporting
// byte-identical package content twice — on any machine, at any time —
// produces a byte-identical archive. Real timestamps would make the archive
// (and therefore its own hash) depend on when it happened to be built.
var deterministicTime = time.Unix(0, 0).UTC()

// Export writes package dir as a deterministic, reproducible tar.gz archive
// to w: the manifest plus every file in the package's standard content
// directories, in sorted path order, with normalized permissions and a
// fixed modification time. Two exports of byte-identical package content
// always produce byte-identical archive bytes.
//
// Export refuses to run against a package that fails Validate — exporting
// is exactly the moment package content starts moving to another machine,
// so it must not ship something with an unresolved secret finding, a
// traversing entrypoint, or a declared-but-missing export.
func Export(dir string, w io.Writer) error {
	report, err := Validate(dir)
	if err != nil {
		return err
	}
	portability := NewPortabilityReport(report)
	if portability.HasBlockers() {
		return fmt.Errorf("package has unresolved portability blockers, refusing to export (%d blocker(s)); run `lineage package validate` for details", len(portability.Blockers))
	}

	files, err := contentFiles(dir)
	if err != nil {
		return fmt.Errorf("read content for export: %w", err)
	}

	gz := gzip.NewWriter(w)
	gz.Header.ModTime = deterministicTime
	tw := tar.NewWriter(gz)

	if err := addTarFile(tw, filepath.Join(dir, ManifestFileName), ManifestFileName); err != nil {
		return err
	}
	for _, rel := range files {
		if err := addTarFile(tw, filepath.Join(dir, filepath.FromSlash(rel)), rel); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("finalize archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("finalize archive compression: %w", err)
	}
	return nil
}

func addTarFile(tw *tar.Writer, absPath, name string) error {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Size:     int64(len(data)),
		Mode:     0o644,
		ModTime:  deterministicTime,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write archive header for %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write archive content for %s: %w", name, err)
	}
	return nil
}
