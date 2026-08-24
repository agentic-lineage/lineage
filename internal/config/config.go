package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-lineage/lineage/internal/atomicfile"
	"gopkg.in/yaml.v3"
)

const (
	DirName        = ".lineage"
	ConfigFileName = "config.yaml"
)

type ProjectConfig struct {
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
