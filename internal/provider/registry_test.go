package provider

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

//	 This gives each future provider one obvious test addition and verifies all
// three parts of its adapter.
//
//	 The test duplicates the registry data: this way, if someone accidentally
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
