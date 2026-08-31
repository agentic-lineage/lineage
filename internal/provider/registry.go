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
//
// RenderSkill and ContextPreamble are optional escape hatches for a
// provider whose native format genuinely can't represent a verbatim
// SKILL.md copy (see docs/decisions/0007-providers-are-a-single-registry-entry.md's
// follow-up on when the registry-entry-only boundary is expected to
// leak). Leaving both nil/empty, as claude and codex do, reproduces
// today's behavior exactly.
type Provider struct {
	Name        string
	SkillsDir   string
	ContextFile string

	// RenderSkill, if set, replaces materialize.Apply's default verbatim
	// directory copy for this provider's skills: it's called once per
	// enabled skill with that skill's staged files, and must return the
	// single file (name and content) to write into SkillsDir instead.
	RenderSkill func(pkgName, skillName string, files map[string][]byte) (filename string, content []byte, err error)

	// ContextPreamble, if set, is written once before the
	// lineage:begin/lineage:end marker block the first time ContextFile
	// is created. Later calls never re-add it: replaceBlock only ever
	// touches the marked region, so whatever precedes it (this preamble)
	// survives every subsequent Apply the same way hand-written content
	// above the markers already does for claude/codex.
	ContextPreamble string
}

var registry = []Provider{
	{Name: "claude", SkillsDir: filepath.Join(".claude", "skills"), ContextFile: "CLAUDE.md"},
	{Name: "codex", SkillsDir: filepath.Join(".agents", "skills"), ContextFile: "AGENTS.md"},
	{
		Name:            "cursor",
		SkillsDir:       filepath.Join(".cursor", "rules"),
		ContextFile:     filepath.Join(".cursor", "rules", "lineage.mdc"),
		RenderSkill:     cursorRenderSkill,
		ContextPreamble: cursorContextPreamble,
	},
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
