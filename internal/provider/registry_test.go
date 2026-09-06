package provider

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

//	This gives each future provider one obvious test addition and verifies all
//
// three parts of its adapter.
//
//	The test duplicates the registry data: this way, if someone accidentally
//
// changes Auggie's path to .augment/rules, the test should fail and force them
// to explain the contract change.
func TestKnownProviders(t *testing.T) {
	want := []Provider{
		{
			Name:        "claude",
			SkillsDir:   filepath.Join(".claude", "skills"),
			ContextFile: "CLAUDE.md",
		},
		{
			Name:        "codex",
			SkillsDir:   filepath.Join(".agents", "skills"),
			ContextFile: "AGENTS.md",
		},
		{
			Name:        "auggie",
			SkillsDir:   filepath.Join(".augment", "skills"),
			ContextFile: "AGENTS.md",
			renderer:    auggieSkillRenderer{},
		},
		{
			Name:            "windsurf",
			SkillsDir:       filepath.Join(".windsurf", "rules"),
			ContextFile:     ".windsurfrules",
			MaterializeOnly: true,
		},
		{
			Name:        "aider",
			SkillsDir:   filepath.Join(".aider", "skills"),
			ContextFile: "CONVENTIONS.md",
			Config:      AiderConfigAdapter{},
		},
		{
			Name:            "cline",
			SkillsDir:       ".clinerules",
			ContextFile:     filepath.Join(".clinerules", "lineage.md"),
			MaterializeOnly: true,
		},
	}

	got := Known()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Known() = %#v, want %#v", got, want)
	}
}

func TestGetKnownProvider(t *testing.T) {
	p, err := Get("codex")
	if err != nil {
		t.Fatalf("Get(codex) error = %v", err)
	}
	if p.SkillsDir == "" || p.ContextFile != "AGENTS.md" {
		t.Fatalf("Get(codex) = %#v", p)
	}
}

func TestGetClineProvider(t *testing.T) {
	p, err := Get("cline")
	if err != nil {
		t.Fatalf("Get(cline) error = %v", err)
	}
	if p.SkillsDir != ".clinerules" || p.ContextFile != filepath.Join(".clinerules", "lineage.md") || !p.MaterializeOnly {
		t.Fatalf("Get(cline) = %#v, want project-scoped Cline paths", p)
	}
}

func TestGetAiderProvider(t *testing.T) {
	p, err := Get("aider")
	if err != nil {
		t.Fatal(err)
	}
	if p.SkillsDir != filepath.Join(".aider", "skills") || p.ContextFile != "CONVENTIONS.md" || p.Config == nil {
		t.Fatalf("Get(aider) = %#v, want Aider conventions and config paths", p)
	}
}

func TestGetWindsurfProvider(t *testing.T) {
	p, err := Get("windsurf")
	if err != nil {
		t.Fatalf("Get(windsurf) error = %v", err)
	}
	if p.SkillsDir != filepath.Join(".windsurf", "rules") || p.ContextFile != ".windsurfrules" || !p.MaterializeOnly {
		t.Fatalf("Get(windsurf) = %#v, want project-scoped Windsurf paths", p)
	}
}

func TestGetUnknownProviderListsKnownNames(t *testing.T) {
	_, err := Get("nope")
	if err == nil {
		t.Fatal("Get(nope) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("Get(nope) error = %q, want it to list known providers", err.Error())
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown("claude") {
		t.Fatal("IsKnown(claude) = false, want true")
	}
	if IsKnown("nope") {
		t.Fatal("IsKnown(nope) = true, want false")
	}
}
