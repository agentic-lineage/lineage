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
	Name        string
	SkillsDir   string
	ContextFile string
}

var registry = []Provider{
	{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"},
	{Name: "codex", SkillsDir: filepath.Join(".agents", "skills"), ContextFile: "AGENTS.md"},
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
