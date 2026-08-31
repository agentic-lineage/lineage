package provider

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownIncludesClaudeAndCodex(t *testing.T) {
	names := map[string]bool{}
	for _, p := range Known() {
		names[p.Name] = true
	}
	if !names["claude"] || !names["codex"] || !names["cursor"] {
		t.Fatalf("Known() = %v, want claude, codex, and cursor present", names)
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

func TestGetCursorProvider(t *testing.T) {
	p, err := Get("cursor")
	if err != nil {
		t.Fatalf("Get(cursor) error = %v", err)
	}
	if p.SkillsDir != filepath.Join(".cursor", "rules") {
		t.Fatalf("Get(cursor).SkillsDir = %q, want .cursor/rules", p.SkillsDir)
	}
	if p.ContextFile != filepath.Join(".cursor", "rules", "lineage.mdc") {
		t.Fatalf("Get(cursor).ContextFile = %q, want .cursor/rules/lineage.mdc", p.ContextFile)
	}
	if p.RenderSkill == nil {
		t.Fatal("Get(cursor).RenderSkill = nil, want a renderer set (Cursor rules need frontmatter, not a verbatim copy)")
	}
	if p.ContextPreamble == "" {
		t.Fatal("Get(cursor).ContextPreamble = \"\", want a frontmatter preamble so lineage.mdc is always loaded")
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
