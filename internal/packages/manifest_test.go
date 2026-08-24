package packages

import (
	"path/filepath"
	"testing"
)

func TestLoadManifestDefaultsMissingSchemaToCurrent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ManifestFileName), "name: legacy-pack\nversion: 1.0.0\n")

	manifest, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if manifest.Schema != CurrentSchema {
		t.Fatalf("Schema = %d, want %d (defaulted)", manifest.Schema, CurrentSchema)
	}
}

func TestLoadManifestRejectsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ManifestFileName), "schema: 99\nname: future-pack\nversion: 1.0.0\n")

	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("LoadManifest() error = nil, want error for unsupported schema")
	}
}

func TestLoadManifestRejectsPathTraversalInName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ManifestFileName), "name: ../../../../tmp/evil\nversion: 1.0.0\n")

	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("LoadManifest() error = nil, want error for a name containing path traversal")
	}
}

func TestLoadManifestRejectsPathTraversalInVersion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ManifestFileName), "name: safe-pack\nversion: ../escaped\n")

	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("LoadManifest() error = nil, want error for a version containing path traversal")
	}
}

func TestLoadManifestRejectsSlashInName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ManifestFileName), "name: nested/name\nversion: 1.0.0\n")

	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("LoadManifest() error = nil, want error for a name containing a path separator")
	}
}

func TestLoadManifestAcceptsSemverStyleVersion(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ManifestFileName), "name: safe-pack\nversion: 1.0.0-beta.1+build.7\n")

	manifest, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v, want a semver-with-metadata version to be accepted", err)
	}
	if manifest.Version != "1.0.0-beta.1+build.7" {
		t.Fatalf("Version = %q, want %q", manifest.Version, "1.0.0-beta.1+build.7")
	}
}

func TestDefaultManifestRoundTripsCapabilities(t *testing.T) {
	dir := t.TempDir()
	manifest := DefaultManifest("cap-pack")
	manifest.Capabilities = Capabilities{
		Filesystem: FilesystemCapabilities{Read: []string{"workspace"}},
		Network:    []string{"github.com"},
	}
	if err := SaveManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Capabilities.Filesystem.Read) != 1 || loaded.Capabilities.Filesystem.Read[0] != "workspace" {
		t.Fatalf("Capabilities.Filesystem.Read = %#v", loaded.Capabilities.Filesystem.Read)
	}
	if len(loaded.Capabilities.Network) != 1 || loaded.Capabilities.Network[0] != "github.com" {
		t.Fatalf("Capabilities.Network = %#v", loaded.Capabilities.Network)
	}
}

// TestLoadManifestDistinguishesAbsentAndExplicitZeroSchema covers #166: an
// absent schema field and an explicit `schema: 0` both unmarshal to 0, so
// the explicit zero — a value that never existed as a real schema — was
// silently coerced to the current schema instead of rejected.
func TestLoadManifestDistinguishesAbsentAndExplicitZeroSchema(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "absent is treated as the pre-schema manifest",
			yaml: "name: legacy-pack\nversion: 1.0.0\n",
		},
		{
			name: "explicit current schema is accepted",
			yaml: "schema: 1\nname: current-pack\nversion: 1.0.0\n",
		},
		{
			name:    "explicit zero is rejected",
			yaml:    "schema: 0\nname: zero-pack\nversion: 1.0.0\n",
			wantErr: true,
		},
		{
			name:    "a future schema is still rejected",
			yaml:    "schema: 2\nname: future-pack\nversion: 1.0.0\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, ManifestFileName), tt.yaml)

			manifest, err := LoadManifest(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadManifest() error = nil, want a rejection for %q", tt.yaml)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadManifest() error = %v", err)
			}
			if manifest.Schema != CurrentSchema {
				t.Errorf("Schema = %d, want %d", manifest.Schema, CurrentSchema)
			}
		})
	}
}
