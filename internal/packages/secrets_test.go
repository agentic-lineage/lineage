package packages

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestScanForSecretsFlagsDeniedFilenames(t *testing.T) {
	root := filepath.Join(t.TempDir(), "leaky-pack")
	if err := InitPackage(root, "leaky-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".env"), "API_KEY=super-secret")
	mustWrite(t, filepath.Join(root, "references", "notes.md"), "# just notes")

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatalf("ScanForSecrets() error = %v", err)
	}
	if !hasFindingForPath(findings, ".env") {
		t.Fatalf("findings = %#v, want a finding for .env", findings)
	}
	if hasFindingForPath(findings, filepath.ToSlash(filepath.Join("references", "notes.md"))) {
		t.Fatalf("findings = %#v, want no finding for an unrelated markdown file", findings)
	}
}

func TestScanForSecretsFlagsPrivateKeyContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "key-pack")
	if err := InitPackage(root, "key-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "references", "oops.txt"), "-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAKCAQ...\n-----END RSA PRIVATE KEY-----\n")

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFindingForPath(findings, filepath.ToSlash(filepath.Join("references", "oops.txt"))) {
		t.Fatalf("findings = %#v, want a finding for the private key content", findings)
	}
}

func TestScanForSecretsFlagsAWSKeyID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "aws-pack")
	if err := InitPackage(root, "aws-pack"); err != nil {
		t.Fatal(err)
	}
	// Split so this fixture's literal bytes never form a real key-shaped
	// string in the source file itself (GitHub push protection flags a
	// contiguous AKIA-prefixed match even in test fixtures).
	fakeAWSKeyID := "AKIA" + "ABCDEFGHIJKLMNOP"
	mustWrite(t, filepath.Join(root, "references", "config.txt"), "aws_access_key_id = "+fakeAWSKeyID+"\n")

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFindingForPath(findings, filepath.ToSlash(filepath.Join("references", "config.txt"))) {
		t.Fatalf("findings = %#v, want a finding for the AWS key ID", findings)
	}
}

func TestScanForSecretsFlagsAWSSessionKeyID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sts-pack")
	if err := InitPackage(root, "sts-pack"); err != nil {
		t.Fatal(err)
	}
	// Split for the same reason as the AKIA fixture above.
	fakeSessionKeyID := "ASIA" + "ABCDEFGHIJKLMNOP"
	mustWrite(t, filepath.Join(root, "references", "session.txt"), "aws_access_key_id = "+fakeSessionKeyID+"\n")

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFindingForPath(findings, filepath.ToSlash(filepath.Join("references", "session.txt"))) {
		t.Fatalf("findings = %#v, want a finding for the temporary STS key ID", findings)
	}
}

func TestScanForSecretsCatchesContentInFilesOverSizeCap(t *testing.T) {
	root := filepath.Join(t.TempDir(), "big-pack")
	if err := InitPackage(root, "big-pack"); err != nil {
		t.Fatal(err)
	}
	// Split so this fixture's literal bytes never form a real key-shaped
	// string in the source file itself (see the AWS key ID test above).
	fakeAWSKeyID := "AKIA" + "ABCDEFGHIJKLMNOP"
	var b strings.Builder
	b.WriteString("aws_access_key_id = " + fakeAWSKeyID + "\n")
	b.WriteString(strings.Repeat("x", 6<<20)) // padding past the 5MB scan cap
	mustWrite(t, filepath.Join(root, "references", "vendor-bundle.txt"), b.String())

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFindingForPath(findings, filepath.ToSlash(filepath.Join("references", "vendor-bundle.txt"))) {
		t.Fatalf("findings = %#v, want a finding for content within the scanned prefix even though the file is over the size cap", findings)
	}
}

func TestScanForSecretsIgnoresSafeContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clean-pack")
	if err := InitPackage(root, "clean-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review\n\nThis skill reviews pull requests.")
	mustWrite(t, filepath.Join(root, "references", "template.csv"), "name,status\n")

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none for a clean package", findings)
	}
}

func TestScanForSecretsNeverIncludesMatchedValue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "redact-pack")
	if err := InitPackage(root, "redact-pack"); err != nil {
		t.Fatal(err)
	}
	secretValue := "AKIA" + "ABCDEFGHIJKLMNOP"
	mustWrite(t, filepath.Join(root, "references", "config.txt"), "key = "+secretValue+"\n")

	findings, err := ScanForSecrets(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.Reason, secretValue) {
			t.Fatalf("finding reason leaked the matched secret value: %q", f.Reason)
		}
	}
}

func hasFindingForPath(findings []SecretFinding, path string) bool {
	for _, f := range findings {
		if f.Path == path {
			return true
		}
	}
	return false
}
