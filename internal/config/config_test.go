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

func TestLoadProjectConfigDefaultsMissingSchemaToCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("workspace: product\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}
	if cfg.Schema != CurrentConfigSchema {
		t.Fatalf("Schema = %d, want %d (defaulted)", cfg.Schema, CurrentConfigSchema)
	}
}

func TestLoadProjectConfigRejectsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("schema: 99\nworkspace: product\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadProjectConfig(path); err == nil {
		t.Fatal("LoadProjectConfig() error = nil, want error for unsupported schema")
	}
}

func TestEnsureGitignoredCreatesFile(t *testing.T) {
	root := t.TempDir()
	if err := EnsureGitignored(root); err != nil {
		t.Fatalf("EnsureGitignored() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != ".lineage/\n" {
		t.Fatalf(".gitignore = %q, want %q", data, ".lineage/\n")
	}
}

func TestEnsureGitignoredAppendsToExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitignored(root); err != nil {
		t.Fatalf("EnsureGitignored() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "node_modules/\n.lineage/\n"
	if string(data) != want {
		t.Fatalf(".gitignore = %q, want %q", data, want)
	}
}

func TestEnsureGitignoredAppendsMissingTrailingNewline(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules/"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitignored(root); err != nil {
		t.Fatalf("EnsureGitignored() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "node_modules/\n.lineage/\n"
	if string(data) != want {
		t.Fatalf(".gitignore = %q, want %q", data, want)
	}
}

func TestEnsureGitignoredIsNoOpWhenAlreadyPresent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	original := "*.log\n.lineage/\nnode_modules/\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitignored(root); err != nil {
		t.Fatalf("EnsureGitignored() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf(".gitignore = %q, want unchanged %q", data, original)
	}
}

func TestEnsureGitignoredRecognizesBareEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	original := ".lineage\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureGitignored(root); err != nil {
		t.Fatalf("EnsureGitignored() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf(".gitignore = %q, want unchanged %q", data, original)
	}
}

func TestSaveProjectConfigRoundTripsSchema(t *testing.T) {
	root := t.TempDir()
	path := ProjectConfigPath(root)

	if err := SaveProjectConfig(path, DefaultProjectConfig()); err != nil {
		t.Fatalf("SaveProjectConfig() error = %v", err)
	}
	got, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}
	if got.Schema != CurrentConfigSchema {
		t.Fatalf("Schema = %d, want %d", got.Schema, CurrentConfigSchema)
	}
}
