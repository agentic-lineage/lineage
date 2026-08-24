package provider

import (
	"strings"
	"testing"
)

func TestCursorRenderSkillProducesValidRule(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: review\ndescription: Use when reviewing a pull request.\n---\n\n# Review\n\nCheck the diff carefully.\n"),
	}

	filename, content, err := cursorRenderSkill("field-notes", "review", files)
	if err != nil {
		t.Fatalf("cursorRenderSkill error = %v", err)
	}
	if filename != "field-notes-review.mdc" {
		t.Fatalf("filename = %q, want field-notes-review.mdc", filename)
	}

	got := string(content)
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("content = %q, want it to start with frontmatter", got)
	}
	if !strings.Contains(got, "description: Use when reviewing a pull request.") {
		t.Fatalf("content = %q, want the skill's description carried into frontmatter", got)
	}
	if !strings.Contains(got, "alwaysApply: false") {
		t.Fatalf("content = %q, want alwaysApply: false", got)
	}
	if !strings.Contains(got, "# Review\n\nCheck the diff carefully.") {
		t.Fatalf("content = %q, want the skill body preserved below the frontmatter", got)
	}
	// The rendered file must have exactly one frontmatter block, not the
	// skill's original one followed by a second one.
	if strings.Count(got, "---\n") != 2 {
		t.Fatalf("content = %q, want exactly one frontmatter block (2 \"---\" delimiters)", got)
	}
}

func TestCursorRenderSkillFailsClosedOnMissingSkillFile(t *testing.T) {
	_, _, err := cursorRenderSkill("field-notes", "review", map[string][]byte{})
	if err == nil {
		t.Fatal("cursorRenderSkill error = nil, want an error for a skill with no SKILL.md")
	}
}

func TestCursorRenderSkillFailsClosedOnMissingFrontmatter(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("# Review\n\nNo frontmatter here.\n"),
	}
	_, _, err := cursorRenderSkill("field-notes", "review", files)
	if err == nil {
		t.Fatal("cursorRenderSkill error = nil, want an error for SKILL.md with no frontmatter")
	}
}

func TestCursorRenderSkillFailsClosedOnMissingDescription(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: review\n---\n\n# Review\n"),
	}
	_, _, err := cursorRenderSkill("field-notes", "review", files)
	if err == nil {
		t.Fatal("cursorRenderSkill error = nil, want an error for frontmatter with no description")
	}
}

func TestSplitFrontmatterNoLeadingDelimiterReturnsBodyOnly(t *testing.T) {
	data := []byte("# Just a heading\n")
	front, body, err := splitFrontmatter(data)
	if err != nil {
		t.Fatalf("splitFrontmatter error = %v", err)
	}
	if front != nil {
		t.Fatalf("front = %q, want nil", front)
	}
	if string(body) != string(data) {
		t.Fatalf("body = %q, want the original data unchanged", body)
	}
}

func TestSplitFrontmatterUnterminatedIsAnError(t *testing.T) {
	_, _, err := splitFrontmatter([]byte("---\ndescription: x\n"))
	if err == nil {
		t.Fatal("splitFrontmatter error = nil, want an error for an unterminated frontmatter block")
	}
}
