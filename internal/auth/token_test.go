package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoadDeleteTokenRoundTrip(t *testing.T) {
	home := t.TempDir()

	got, err := LoadToken(home)
	if err != nil {
		t.Fatalf("LoadToken() before any save: error = %v", err)
	}
	if got != "" {
		t.Fatalf("LoadToken() before any save = %q, want empty", got)
	}

	if err := SaveToken(home, "gho_abc123"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	got, err = LoadToken(home)
	if err != nil {
		t.Fatalf("LoadToken() error = %v", err)
	}
	if got != "gho_abc123" {
		t.Errorf("LoadToken() = %q, want gho_abc123", got)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(TokenPath(home))
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			t.Errorf("token file mode = %o, want no group/other permission bits", mode)
		}
	}

	if err := DeleteToken(home); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	got, err = LoadToken(home)
	if err != nil {
		t.Fatalf("LoadToken() after delete: error = %v", err)
	}
	if got != "" {
		t.Errorf("LoadToken() after delete = %q, want empty", got)
	}

	// Deleting an already-absent token is not an error.
	if err := DeleteToken(home); err != nil {
		t.Fatalf("DeleteToken() on an already-deleted token: error = %v", err)
	}
}

func TestTokenPathIsUnderHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "lineage-home")
	if got, want := TokenPath(home), filepath.Join(home, "github_token"); got != want {
		t.Errorf("TokenPath(%q) = %q, want %q", home, got, want)
	}
}
