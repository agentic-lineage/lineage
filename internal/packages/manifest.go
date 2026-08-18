package packages

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const ManifestFileName = "lineage.yaml"

// CurrentSchema is the only lineage.yaml schema this build understands.
// schema describes how to interpret the manifest; version describes the
// package itself — the two change independently.
const CurrentSchema = 1

type Manifest struct {
	Schema       int          `yaml:"schema"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Description  string       `yaml:"description"`
	Exports      Exports      `yaml:"exports"`
	Requires     Requires     `yaml:"requires"`
	Entrypoints  Entrypoints  `yaml:"entrypoints"`
	Capabilities Capabilities `yaml:"capabilities"`
}

type Exports struct {
	Agents    []string `yaml:"agents"`
	Workflows []string `yaml:"workflows"`
}

type Requires struct {
	Skills []string `yaml:"skills"`
}

type Entrypoints struct {
	Claude string `yaml:"claude"`
	Codex  string `yaml:"codex"`
}

// Capabilities is a purely declarative statement of what a package wants
// access to — printed by lineage package validate and the enable-time plan
// so a receiver can see it before enabling, not enforced by this build.
type Capabilities struct {
	Filesystem FilesystemCapabilities `yaml:"filesystem"`
	Network    []string               `yaml:"network"`
}

type FilesystemCapabilities struct {
	Read []string `yaml:"read"`
}

func DefaultManifest(name string) Manifest {
	return Manifest{
		Schema:      CurrentSchema,
		Name:        name,
		Version:     "0.1.0",
		Description: "A shareable Lineage agent package.",
		Exports: Exports{
			Agents:    []string{},
			Workflows: []string{},
		},
		Requires: Requires{
			Skills: []string{},
		},
		Entrypoints: Entrypoints{},
		Capabilities: Capabilities{
			Filesystem: FilesystemCapabilities{Read: []string{}},
			Network:    []string{},
		},
	}
}

func LoadManifest(dir string) (Manifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if manifest.Name == "" {
		return Manifest{}, fmt.Errorf("manifest %s missing name", path)
	}
	if manifest.Version == "" {
		manifest.Version = "0.1.0"
	}
	// schema 0 means the manifest predates the schema field; treat that as
	// schema 1, the only schema that ever existed before it was added.
	if manifest.Schema == 0 {
		manifest.Schema = CurrentSchema
	}
	if manifest.Schema != CurrentSchema {
		return Manifest{}, fmt.Errorf("manifest %s declares schema %d, but this build only understands schema %d", path, manifest.Schema, CurrentSchema)
	}
	return manifest, nil
}

func SaveManifest(dir string, manifest Manifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create package directory: %w", err)
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}
