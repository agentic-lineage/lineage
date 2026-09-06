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
	// An absent schema field and an explicit `schema: 0` both leave Schema
	// at zero when decoded without defaults. Probe the field as a pointer so
	// only the absent legacy field defaults to schema 1; zero was never a
	// valid schema and must be rejected like any other unsupported value.
	var probe struct {
		Schema *int `yaml:"schema"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return ProjectConfig{}, fmt.Errorf("parse project config %s: %w", path, err)
	}
	if probe.Schema == nil {
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
	// ProjectConfig values constructed by callers before schema versioning have
	// the Go zero value here. Current writers must never persist an explicit
	// schema zero, because loaders correctly reject it as unsupported.
	if cfg.Schema == 0 {
		cfg.Schema = CurrentConfigSchema
	}
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
// already matches once surrounding whitespace is trimmed: either
// GitignoreEntry itself or a bare ".lineage". It is safe to call on every
// successful enable without duplicating entries.
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
