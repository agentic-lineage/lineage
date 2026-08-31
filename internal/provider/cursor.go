package provider

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// cursorContextPreamble is written once, before materialize's generated
// lineage:begin/lineage:end block, the first time Cursor's ContextFile is
// created. Cursor only ever loads a .mdc file's content if it has valid
// frontmatter (see cursorRenderSkill); alwaysApply: true is what makes
// this the equivalent of CLAUDE.md/AGENTS.md, which Claude/Codex read
// unconditionally.
const cursorContextPreamble = `---
description: Lineage-managed project context (active packages, agents, policies, and workflows)
alwaysApply: true
---

`

// cursorRuleFrontmatter is the YAML frontmatter every .mdc file under
// .cursor/rules/ needs to be recognized as a rule at all — Cursor
// silently ignores a .mdc file with no frontmatter, and a plain .md file
// entirely. globs is deliberately omitted: skills aren't scoped to a
// file-pattern, so leaving it unset (rather than an empty list) matches
// how Cursor treats a description-only, non-file-scoped rule.
type cursorRuleFrontmatter struct {
	Description string `yaml:"description"`
	AlwaysApply bool   `yaml:"alwaysApply"`
}

// cursorRenderSkill turns one staged skill into a Cursor rule file:
// <pkg>-<skill>.mdc, with frontmatter carrying the skill's own
// description (every SKILL.md already declares one - see the guardrail
// skills under .agents/skills/*/SKILL.md in this repo) and the skill's
// body unchanged below it. alwaysApply is always false: Cursor decides
// relevance from the description, the same activation model the skill's
// description already serves for Claude/Codex.
//
// Fails closed rather than materializing a rule Cursor will silently
// never load: a skill with no SKILL.md, no frontmatter, or no
// description is a package-authoring problem worth surfacing at
// `lineage run cursor` time, not a best-effort file to skip past.
func cursorRenderSkill(pkgName, skillName string, files map[string][]byte) (string, []byte, error) {
	data, ok := files["SKILL.md"]
	if !ok {
		return "", nil, fmt.Errorf("skill %q (package %q) has no SKILL.md; Cursor rules require one", skillName, pkgName)
	}

	front, body, err := splitFrontmatter(data)
	if err != nil {
		return "", nil, fmt.Errorf("skill %q (package %q): %w", skillName, pkgName, err)
	}
	if front == nil {
		return "", nil, fmt.Errorf("skill %q (package %q): SKILL.md has no frontmatter; Cursor rules require a description", skillName, pkgName)
	}

	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return "", nil, fmt.Errorf("skill %q (package %q): parse SKILL.md frontmatter: %w", skillName, pkgName, err)
	}
	if strings.TrimSpace(meta.Description) == "" {
		return "", nil, fmt.Errorf("skill %q (package %q): SKILL.md frontmatter has no description; Cursor rules require one", skillName, pkgName)
	}

	fm, err := yaml.Marshal(cursorRuleFrontmatter{Description: meta.Description, AlwaysApply: false})
	if err != nil {
		return "", nil, fmt.Errorf("skill %q (package %q): render Cursor rule frontmatter: %w", skillName, pkgName, err)
	}

	// No blank line inserted here: body already carries whatever spacing
	// followed the skill's own closing "---" (splitFrontmatter trims
	// exactly one leading newline, not the blank line after it), so
	// adding another would double it up.
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(fm)
	out.WriteString("---\n")
	out.Write(body)

	return pkgName + "-" + skillName + ".mdc", out.Bytes(), nil
}

// splitFrontmatter separates a leading "---"-delimited YAML block from the
// rest of data, mirroring the convention SKILL.md/WORKFLOW.md already use
// (see internal/packages/workflow.go's workflowFrontmatter). Content with
// no leading "---" line returns (nil, data, nil): no frontmatter, not an
// error, since a bare SKILL.md is a package-authoring problem the caller
// decides how to react to, not this function's job to reject.
func splitFrontmatter(data []byte) (front, body []byte, err error) {
	const delim = "---"
	text := string(data)
	if !strings.HasPrefix(text, delim+"\n") {
		return nil, data, nil
	}
	rest := text[len(delim)+1:]
	end := strings.Index(rest, "\n"+delim)
	if end == -1 {
		return nil, nil, fmt.Errorf("unterminated frontmatter (missing closing %q)", delim)
	}
	front = []byte(rest[:end])
	body = []byte(strings.TrimPrefix(rest[end+len("\n"+delim):], "\n"))
	return front, body, nil
}
