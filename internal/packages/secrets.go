package packages

import (
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

// maxScannedFileSize caps how much of a file's content gets pattern-matched.
// Secret-shaped strings are text and small; skipping large files avoids
// reading big binaries into memory for no benefit.
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
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),         // AWS access key ID
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`), // GitHub token prefixes (ghp_, gho_, ghu_, ghs_, ghr_)
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
		if statErr != nil || info.Size() > maxScannedFileSize {
			return nil
		}
		data, readErr := os.ReadFile(path)
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
