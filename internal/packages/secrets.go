package packages

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SecretFinding is one file that a secret scan flagged, and why.
type SecretFinding struct {
	Path   string // relative to the package root, forward-slashed
	Reason string
}

// maxScannedFileSize caps how much of a file's content gets pattern-matched:
// the first maxScannedFileSize bytes, not the whole file. Secret-shaped
// strings are text, small, and tend to appear early in a file (env var
// assignments, key headers), so scanning a bounded prefix catches them in
// large files too, without reading an arbitrarily large file fully into
// memory.
const maxScannedFileSize = 5 << 20 // 5MB

// deniedFilenames are exact, case-insensitive filename matches that are
// never safe to distribute in a package: common secret/credential store
// filenames. This is deliberately a small, documented, testable list for
// v1 rather than a general-purpose secret-scanning engine.
var deniedFilenames = []string{
	".env",
	".npmrc",
	".netrc",
	".pgpass",
	"credentials",
	"credentials.json",
	"id_rsa",
	"id_dsa",
	"id_ecdsa",
	"id_ed25519",
}

// deniedExtensions are file extensions that commonly hold private key or
// credential material.
var deniedExtensions = []string{".pem", ".key", ".pfx", ".p12"}

// secretContentPatterns are high-confidence content signatures: things that
// are essentially never present in legitimate package source material.
var secretContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`), // Google API key
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	// AWS access key ID: AKIA is a long-lived user key, ASIA a temporary
	// STS/assumed-role session key.
	regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`),   // GitHub token prefixes (ghp_, gho_, ghu_, ghs_, ghr_)
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}`), // GitHub fine-grained personal access token
}

// ScanForSecrets walks a package directory and flags files that look like
// they contain secrets or credentials, by filename pattern or by
// high-confidence content pattern. It never returns the matched content
// itself — only the path and a human-readable reason — so a caller can
// safely print findings without risking a secret ending up in command
// output.
func ScanForSecrets(dir string) ([]SecretFinding, error) {
	var findings []SecretFinding

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		if reason, matched := matchesDeniedFilename(d.Name()); matched {
			findings = append(findings, SecretFinding{Path: relSlash, Reason: reason})
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		data, readErr := readScanPrefix(path, info.Size())
		if readErr != nil {
			return nil
		}
		if reason, matched := matchesSecretContent(data); matched {
			findings = append(findings, SecretFinding{Path: relSlash, Reason: reason})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}

// readScanPrefix reads up to maxScannedFileSize bytes from the start of
// path. size is the file's already-known length, used only to avoid
// allocating a buffer larger than the file actually is; a short read (the
// file being smaller than size, or than the cap) is not an error - it's
// what happens for every file smaller than the cap, which is most of them.
func readScanPrefix(path string, size int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	limit := size
	if limit > maxScannedFileSize {
		limit = maxScannedFileSize
	}
	buf := make([]byte, limit)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

func matchesDeniedFilename(name string) (string, bool) {
	lower := strings.ToLower(name)
	for _, denied := range deniedFilenames {
		if lower == denied {
			return "filename matches denylisted credential file " + denied, true
		}
	}
	if strings.HasPrefix(lower, ".env.") {
		return "filename matches denylisted .env.* pattern", true
	}
	ext := strings.ToLower(filepath.Ext(lower))
	for _, denied := range deniedExtensions {
		if ext == denied {
			return "file extension " + denied + " commonly holds private key material", true
		}
	}
	return "", false
}

func matchesSecretContent(data []byte) (string, bool) {
	for _, pattern := range secretContentPatterns {
		if pattern.Match(data) {
			return "content matches a high-confidence secret pattern (" + pattern.String() + ")", true
		}
	}
	return "", false
}
