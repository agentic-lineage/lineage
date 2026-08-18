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
