package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := ProjectConfigPath(root)
	cfg := ProjectConfig{
		Workspace:       "product",
		EnabledPackages: []string{"./agent-pack"},
		Providers: map[string]Provider{
			"claude": {Binary: "/bin/echo", Args: []string{"hello"}},
		},
	}

	if err := SaveProjectConfig(path, cfg); err != nil {
		t.Fatalf("SaveProjectConfig() error = %v", err)
	}
	got, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if got.Workspace != "product" {
		t.Fatalf("Workspace = %q, want product", got.Workspace)
	}
	if len(got.EnabledPackages) != 1 || got.EnabledPackages[0] != "./agent-pack" {
		t.Fatalf("EnabledPackages = %#v", got.EnabledPackages)
	}
	if got.Providers["claude"].Binary != "/bin/echo" {
		t.Fatalf("Providers[claude].Binary = %q", got.Providers["claude"].Binary)
	}
}

func TestFindProjectConfigWalksUp(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveProjectConfig(ProjectConfigPath(root), DefaultProjectConfig()); err != nil {
		t.Fatal(err)
	}

	found, err := FindProjectConfig(child)
	if err != nil {
		t.Fatalf("FindProjectConfig() error = %v", err)
	}
	if found.Root != root {
		t.Fatalf("Root = %q, want %q", found.Root, root)
	}
}

func TestFindProjectConfigMissing(t *testing.T) {
	_, err := FindProjectConfig(t.TempDir())
	if err == nil {
		t.Fatal("FindProjectConfig() error = nil, want error")
	}
}

// TestLoadProjectConfigNormalizesExplicitEmptyCollections covers #168: an
// explicit `enabled_packages:` with no value unmarshals to nil, overwriting
// the empty slice DefaultProjectConfig seeded. The two sibling map fields
// were already guarded; this one was not.
func TestLoadProjectConfigNormalizesExplicitEmptyCollections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("enabled_packages:\nprovider_preferences:\nproviders:\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}
	if cfg.EnabledPackages == nil {
		t.Error("EnabledPackages = nil, want an empty slice like the sibling map fields get")
	}
	if cfg.ProviderPreferences == nil {
		t.Error("ProviderPreferences = nil, want an empty map")
	}
	if cfg.Providers == nil {
		t.Error("Providers = nil, want an empty map")
	}
}
