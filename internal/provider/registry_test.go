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
	if !names["claude"] || !names["codex"] {
		t.Fatalf("Known() = %v, want claude and codex present", names)
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
	if p.SkillsDir != ".clinerules" || p.ContextFile != filepath.Join(".clinerules", "lineage.md") {
		t.Fatalf("Get(cline) = %#v, want project-scoped Cline paths", p)
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
