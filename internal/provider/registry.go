package provider

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Provider is the complete, self-contained description of one supported
// agent provider: its name, and where package materialization stages
// content for it (a skills directory and a generated-context file, both
// relative to the project root). Adding a new provider means adding one
// entry to registry below — no other package (internal/runtime,
// internal/packages, internal/cli) should ever special-case a provider
// name. Providers sit on top of the provider-neutral core, not inside it.
type Provider struct {
	Name            string
	SkillsDir       string
	ContextFile     string
	MaterializeOnly bool
	Config          ConfigAdapter
}

// ConfigState records a provider-specific project configuration edit so the
// materializer can reverse only the content it added. Adapters own the file
// syntax; the core only persists and passes this state through.
type ConfigState struct {
	FileExisted bool     `json:"file_existed"`
	CreatedFile bool     `json:"created_file"`
	Original    []string `json:"original,omitempty"`
	Managed     []string `json:"managed,omitempty"`
}

// ConfigAdapter describes optional project-scoped configuration that connects
// a provider's generated context file to its native configuration.
type ConfigAdapter interface {
	Ensure(projectRoot string) (ConfigState, error)
	Remove(projectRoot string, state ConfigState) error
	NeedsApproval(projectRoot string, state ConfigState, desired bool) (bool, error)
}

var registry = []Provider{
	{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"},
	{Name: "codex", SkillsDir: filepath.Join(".agents", "skills"), ContextFile: "AGENTS.md"},
	{Name: "windsurf", SkillsDir: filepath.Join(".windsurf", "rules"), ContextFile: ".windsurfrules", MaterializeOnly: true},
	{Name: "aider", SkillsDir: filepath.Join(".aider", "skills"), ContextFile: "CONVENTIONS.md", Config: AiderConfigAdapter{}},
}

// Known returns every registered provider, in registration order.
func Known() []Provider {
	return append([]Provider(nil), registry...)
}

// IsKnown reports whether name is a registered provider.
func IsKnown(name string) bool {
	_, err := Get(name)
	return err == nil
}

// Get returns the registered Provider for name, or an error naming every
// known provider if name isn't registered.
func Get(name string) (Provider, error) {
	for _, p := range registry {
		if p.Name == name {
			return p, nil
		}
	}
	names := make([]string, 0, len(registry))
	for _, p := range registry {
		names = append(names, p.Name)
	}
	return Provider{}, fmt.Errorf("unknown provider %q (known providers: %s)", name, strings.Join(names, ", "))
}
