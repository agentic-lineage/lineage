package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-lineage/lineage/internal/atomicfile"
	"gopkg.in/yaml.v3"
)

const (
	DirName        = ".lineage"
	ConfigFileName = "config.yaml"
)

// CurrentConfigSchema is the only config.yaml schema this build
// understands. Mirrors packages.CurrentSchema (see docs/decisions/0005):
// schema describes how to interpret the file, so a future incompatible
// change to config.yaml's shape has a clean way to say so instead of a
// parser guessing.
const CurrentConfigSchema = 1

type ProjectConfig struct {
	Schema              int                 `yaml:"schema"`
	Workspace           string              `yaml:"workspace"`
	EnabledPackages     []string            `yaml:"enabled_packages"`
	ProviderPreferences map[string]string   `yaml:"provider_preferences"`
	Providers           map[string]Provider `yaml:"providers"`
}

type Provider struct {
	Binary string   `yaml:"binary"`
	Args   []string `yaml:"args"`
}

type FoundProjectConfig struct {
	Root   string
	Path   string
	Config ProjectConfig
}

// ErrProjectConfigNotFound is returned by FindProjectConfig when no
// .lineage/config.yaml exists at the start directory or any parent.
// Callers that can legitimately initialize a new project config (such as
// `lineage enable`) should check for this with errors.Is and fall back to
// creating one, rather than treating every non-nil error the same way.
var ErrProjectConfigNotFound = errors.New("no " + DirName + "/" + ConfigFileName + " found in this directory or any parent")

func DefaultProjectConfig() ProjectConfig {
	return ProjectConfig{
		Schema:              CurrentConfigSchema,
		EnabledPackages:     []string{},
		ProviderPreferences: map[string]string{},
		Providers:           map[string]Provider{},
	}
}

func FindProjectConfig(start string) (FoundProjectConfig, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return FoundProjectConfig{}, fmt.Errorf("resolve start directory: %w", err)
	}

	for {
		path := filepath.Join(current, DirName, ConfigFileName)
		cfg, err := LoadProjectConfig(path)
		if err == nil {
			return FoundProjectConfig{Root: current, Path: path, Config: cfg}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return FoundProjectConfig{}, err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return FoundProjectConfig{}, fmt.Errorf("%w; run lineage enable <package-path-or-id> from a project first", ErrProjectConfigNotFound)
		}
		current = parent
	}
}

func LoadProjectConfig(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectConfig{}, err
	}

	cfg := DefaultProjectConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, fmt.Errorf("parse project config %s: %w", path, err)
	}
	// schema 0 means the config predates the schema field; treat that as
	// schema 1, the only schema that ever existed before it was added -
	// same rule packages.LoadManifest uses for lineage.yaml's schema field.
	if cfg.Schema == 0 {
		cfg.Schema = CurrentConfigSchema
	}
	if cfg.Schema != CurrentConfigSchema {
		return ProjectConfig{}, fmt.Errorf("config %s declares schema %d, but this build only understands schema %d", path, cfg.Schema, CurrentConfigSchema)
	}
	if cfg.ProviderPreferences == nil {
		cfg.ProviderPreferences = map[string]string{}
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]Provider{}
	}
	if cfg.EnabledPackages == nil {
		cfg.EnabledPackages = []string{}
	}
	return cfg, nil
}

func SaveProjectConfig(path string, cfg ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode project config: %w", err)
	}
	if err := atomicfile.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write project config: %w", err)
	}
	return nil
}

func ProjectConfigPath(root string) string {
	return filepath.Join(root, DirName, ConfigFileName)
}

// GitignoreEntry is the line EnsureGitignored writes to a project's
// .gitignore. Project-level .lineage/ mixes machine-local state
// (config.yaml's provider binary paths) with regenerable cache
// (materialized-<provider>.json) and is never meant to be committed to a
// receiver's repo - see docs/decisions for the full rationale.
const GitignoreEntry = DirName + "/"

// EnsureGitignored appends GitignoreEntry to projectRoot's .gitignore,
// creating the file if it doesn't exist yet. It is a no-op if some line
// already matches once surrounding whitespace is trimmed (whether that's
// GitignoreEntry itself, a bare ".lineage", or a broader ignore a receiver
// already wrote by hand), so it's safe to call every time a project's
// .lineage/ is first created without duplicating entries or fighting a
// receiver who deliberately removed it.
func EnsureGitignored(projectRoot string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == GitignoreEntry || trimmed == DirName {
			return nil
		}
	}

	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += GitignoreEntry + "\n"

	if err := atomicfile.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
