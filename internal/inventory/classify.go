package inventory

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// instructionFileNames are exact basenames that always classify as
// instruction material, wherever they appear in the tree — these are the
// files an agent actually reads to know what to do.
var instructionFileNames = map[string]bool{
	"CLAUDE.md":   true,
	"AGENTS.md":   true,
	"SKILL.md":    true,
	"WORKFLOW.md": true,
}

// setupFilePrefixes are case-sensitive basename prefixes that classify as
// setup material: install/readme notes, not behavior itself.
var setupFilePrefixes = []string{"README", "SETUP", "INSTALL"}

// packageMetadataFileNames are exact basenames that classify as existing
// package metadata a source tree might already carry.
var packageMetadataFileNames = map[string]bool{
	"lineage.yaml": true,
	"package.yaml": true,
}

// executableExtensions classify as executable helpers by extension.
var executableExtensions = map[string]bool{
	".sh":  true,
	".py":  true,
	".js":  true,
	".ts":  true,
	".rb":  true,
	".ps1": true,
}

// referenceExtensions classify as reference/data material by extension,
// when not otherwise claimed by a more specific rule.
var referenceExtensions = map[string]bool{
	".pdf":  true,
	".csv":  true,
	".json": true,
	".yaml": true,
	".yml":  true,
	".txt":  true,
}

// languageByExtension is a best-effort, hand-rolled extension-to-language
// table — deliberately small and explicit rather than pulling in a
// mimetype/language-detection dependency, matching the rest of this
// codebase's style.
var languageByExtension = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".ts":   "typescript",
	".sh":   "shell",
	".rb":   "ruby",
	".ps1":  "powershell",
	".md":   "markdown",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
}

// classify assigns an ArtifactKind and a base reason to a file by its
// relative path, using name/extension heuristics only. A file that matches
// no rule is KindUnknown with an honest "no classification rule matched"
// reason rather than a forced guess — ambiguity is surfaced, not resolved,
// here.
func classify(rel string) (ArtifactKind, string) {
	base := path.Base(rel)
	ext := path.Ext(base)
	dir := path.Dir(rel)

	if instructionFileNames[base] {
		return KindInstruction, fmt.Sprintf("filename %q is a recognized instruction file", base)
	}
	if packageMetadataFileNames[base] {
		return KindPackageMetadata, fmt.Sprintf("filename %q is existing package metadata", base)
	}
	// A markdown file sitting directly in a skill's or workflow's own
	// top-level folder is very likely that unit's instruction content even
	// when it doesn't use the canonical SKILL.md/WORKFLOW.md filename —
	// e.g. skills/foo/README.md or workflows/deploy.md from a workspace
	// that hasn't adopted Lineage's naming convention yet. Directory
	// location outranks a generic README/SETUP prefix match here: a README
	// directly inside a skill folder means something different from a
	// README at the workspace root.
	//
	// Deliberately shallow: only skills/<name>/*.md and workflows/*.md or
	// workflows/<name>/*.md qualify — a markdown file nested any deeper
	// (skills/<name>/templates/*.md, skills/<name>/outputs/*.md) is
	// auxiliary material, not the skill's own instructions, and matching it
	// anyway was a real false-positive found by testing this rule against a
	// real repo's skills tree (eval logs and templates got misclassified as
	// instruction). Deeper markdown falls through to the generic unknown
	// markdown case below instead of a guess.
	if ext == ".md" && isSkillOrWorkflowOwnFile(dir) {
		return KindInstruction, fmt.Sprintf("markdown file is this skill/workflow/agent's own top-level file (%s), likely instruction content despite non-canonical filename", dir)
	}
	for _, prefix := range setupFilePrefixes {
		if strings.HasPrefix(base, prefix) {
			return KindSetupMaterial, fmt.Sprintf("filename %q matches setup-note prefix %q", base, prefix)
		}
	}
	if inReferencesDir(dir) {
		return KindReference, fmt.Sprintf("located under a references directory (%s)", dir)
	}
	if executableExtensions[ext] {
		return KindExecutableHelper, fmt.Sprintf("extension %q is a recognized script/executable type", ext)
	}
	if referenceExtensions[ext] {
		return KindReference, fmt.Sprintf("extension %q is a recognized reference/data type", ext)
	}
	if ext == ".md" {
		return KindUnknown, "unrecognized markdown file — possible instruction content, review manually"
	}
	return KindUnknown, "no classification rule matched"
}

// inReferencesDir reports whether dir (a forward-slashed relative
// directory) is, or is nested under, a directory literally named
// "references" or "refs".
func inReferencesDir(dir string) bool {
	return underDir(dir, "references", "refs")
}

// underDir reports whether dir (a forward-slashed relative directory) is,
// or is nested under, a directory whose name matches any of names.
func underDir(dir string, names ...string) bool {
	for _, seg := range strings.Split(dir, "/") {
		for _, name := range names {
			if seg == name {
				return true
			}
		}
	}
	return false
}

// knownProviderRoots are directory prefixes real workspaces commonly nest
// skills/workflows/agents content under, mirroring internal/provider's
// registry (SkillsDir ".claude/skills" for claude, ".agents/skills" for
// codex) — found necessary by testing against a real repo whose agent
// definitions live at .claude/agents/*.md, which the bare (unprefixed)
// shape below doesn't reach on its own.
var knownProviderRoots = []string{".claude", ".agents"}

// isSkillOrWorkflowOwnFile reports whether dir is exactly one of the shapes
// that mean "this skill's/workflow's/agent's own top-level file", not
// merely nested somewhere underneath one, optionally under a known
// provider root (see knownProviderRoots): "workflows" or "agents"
// themselves (a flat workflows/<name>.md or agents/<name>.md file,
// matching how Lineage's own agents/ content dir works — flat files, no
// per-agent subfolder), or a single named child of "skills" or "workflows"
// (skills/<name> or workflows/<name>, a per-unit folder). Anything
// deeper — skills/<name>/templates, skills/<name>/outputs, and so on — is
// auxiliary material colocated with the skill, not the skill's own
// instructions, and intentionally does not match here (found to matter by
// testing against a real repo whose eval logs and templates live exactly
// one level deeper than this).
func isSkillOrWorkflowOwnFile(dir string) bool {
	segs := strings.Split(dir, "/")
	for _, prefix := range knownProviderRoots {
		if len(segs) > 1 && segs[0] == prefix {
			segs = segs[1:]
			break
		}
	}
	if len(segs) == 1 && (segs[0] == "workflows" || segs[0] == "agents") {
		return true
	}
	if len(segs) == 2 && (segs[0] == "skills" || segs[0] == "workflows") {
		return true
	}
	return false
}

// languageFor returns a best-effort language label by extension, or "" if
// the extension isn't in the known table.
func languageFor(rel string) string {
	return languageByExtension[path.Ext(rel)]
}

// crossReference implements pass 2: every markdown file's content is
// scanned line-by-line for path references that name another entry — its full
// relative path, or a suffix of it down to the bare filename (see
// classifyReference). Each (line, target) pair becomes one
// Citation — a directed FromPath -> ToPath edge — recorded on both the
// citer's Mentions and the target's ReferencedBy, so the graph reads from
// either end. A line naming the same target twice yields one citation,
// anchored at the first match.
//
// Eligibility to cite is based on being markdown (prose), not on Kind: a
// real messy workspace has plenty of instruction-shaped .md files that
// don't match the canonical CLAUDE.md/AGENTS.md/SKILL.md/WORKFLOW.md names
// or a skills/workflows directory, and classify() correctly leaves those as
// KindUnknown rather than guessing — but they still literally name other
// files in their prose, and that's real, citable evidence regardless of
// whether classify() could confidently label the citing file itself.
//
// This is deliberately plain string matching, not any form of semantic
// analysis — see the package doc comment for why that boundary is
// intentional.
//
// Cost is O(markdown lines x candidates), and each markdown file is read
// fully into memory. That is an accepted v1 tradeoff: the expected input is a
// single agent workspace, tens to low hundreds of files. Replacing the scan
// with a token index is the obvious lever if that stops holding — note that
// the citation order for a line falls out of the sorted candidate walk (see
// buildCandidates), so an index-based rewrite has to sort each line's matches
// explicitly to stay deterministic.
func crossReference(root string, relPaths []string, entries map[string]*Entry) error {
	candidates := buildCandidates(relPaths)

	// buildCandidates already worked out which basenames collide; surface that
	// on the entry rather than counting basenames a second time.
	for _, c := range candidates {
		if c.baseShared {
			entries[c.path].AmbiguousBasename = true
		}
	}

	for _, rel := range relPaths {
		entry := entries[rel]
		if path.Ext(rel) != ".md" {
			continue
		}

		full := filepath.Join(root, filepath.FromSlash(rel))
		lines, err := readLines(full)
		if err != nil {
			return fmt.Errorf("read %s for citation scan: %w", rel, err)
		}
		citerDir := citerDirOf(rel)

		for lineNo, line := range lines {
			for _, target := range candidates {
				if target.path == rel {
					continue // a file never cites itself
				}
				at, kind, matched := findReference(line, target, citerDir)
				if at < 0 {
					continue
				}
				citation := Citation{
					FromPath:  rel,
					ToPath:    target.path,
					Line:      lineNo + 1,
					Column:    at + 1,
					MatchKind: kind,
					AsWritten: matched,
					Snippet:   snippetOf(line, at, len(matched)),
				}
				entry.Mentions = append(entry.Mentions, citation)
				if targetEntry, ok := entries[target.path]; ok {
					targetEntry.ReferencedBy = append(targetEntry.ReferencedBy, citation)
				}
			}
		}
	}
	return nil
}

// isPathTokenByte reports whether b can appear inside a single path segment.
//
// Every byte of a multi-byte UTF-8 sequence is >= 0x80, so those count too: an
// accented letter is part of the name, and reading it as a separator breaks
// both directions. It invents citations, where a name butting straight against
// the match ("añnotas.md" naming "notas.md") looks like a clean word break;
// and, now that widening decides matches, it loses real ones by truncating a
// path at its non-ASCII segment, so "docs/café/data.csv" would widen to only
// "/data.csv" and fail to match itself.
func isPathTokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		b == '_' || b == '-' || b >= 0x80
}

// pathTokenAround widens [i, i+n) to the whole path token the prose wrote
// around it, and is what turns a bare substring hit into a claim that can be
// checked. A "." belongs to the token only when a name follows it, so ".bak"
// in "deploy.sh.bak" is pulled in while the period ending "run deploy.sh." is
// left out.
func pathTokenAround(line string, i, n int) (int, int) {
	start := i
	for start > 0 {
		if b := line[start-1]; !isPathTokenByte(b) && b != '/' && b != '.' {
			break
		}
		start--
	}
	end := i + n
	for end < len(line) {
		b := line[end]
		if isPathTokenByte(b) || b == '/' {
			end++
			continue
		}
		if b == '.' && end+1 < len(line) && (isPathTokenByte(line[end+1]) || line[end+1] == '/') {
			end++
			continue
		}
		break
	}
	return start, end
}

// classifyReference decides whether written — one path token lifted from the
// prose — names target, and how strongly. citerDir is the directory the
// citing markdown file sits in (forward-slashed, "" for the workspace root),
// needed to resolve navigation exactly rather than just bound it.
//
// A written reference containing a leading "." or ".." segment is navigated:
// the author asserted one specific, resolvable location, so it is resolved
// against citerDir and must match target.path exactly — no partial credit.
// "../scripts/deploy.sh" written in skills/foo/SKILL.md resolves to
// skills/scripts/deploy.sh, an ancestor directory, not the workspace root;
// treating it as a fuzzy suffix (as an earlier version of this function did)
// would cite an unrelated root-level scripts/deploy.sh instead. Climbing past
// the root (more ".." than citerDir has segments) is rejected outright: that
// names something outside the tree, so it cannot be evidence about a file
// inside it.
//
// An un-navigated reference makes a fuzzier claim — "this is somewhere in the
// tree, possibly a shorthand" — and is compared as a segment-aligned suffix of
// target's path instead. Equality is the strongest claim (MatchPath); a
// proper suffix ("foo/run.sh", or a bare "run.sh") is weaker (MatchBasename).
// Anything else names a *different* file that merely shares a tail —
// "archive/references/data.csv" is not a mention of "references/data.csv",
// and "scripts/build/output.txt" is not a mention of "scripts/build".
//
// Two guards apply only to the un-navigated MatchBasename case, since an exact
// navigated resolution is inherently unambiguous:
//   - A proper suffix only identifies one file when the basename is unique in
//     the tree, so a shared basename demands an exact path instead —
//     "y/report.json" must not fan out across "x/y/report.json" and
//     "z/y/report.json".
//   - A single bare segment — no "/" anywhere in written — can only cite a
//     target whose basename contains a ".". Stripped of any path separator,
//     "run" looks exactly like the English word and nothing like a
//     deliberate reference, whereas "run.sh" essentially never occurs by
//     accident. This applies even when the target lives at the workspace
//     root and a bare match is therefore also an exact-path match ("run"
//     citing root-level file "run") — root placement doesn't make a bare
//     word any less ambiguous with prose. A written reference with a "/"
//     anywhere in it, or an explicit "."/".." prefix, is never blocked by
//     this: a path separator is not something English prose produces by
//     coincidence.
func classifyReference(written string, target candidate, citerDir string) (MatchKind, bool) {
	segs := strings.Split(written, "/")
	navigated, ups := false, 0
	for len(segs) > 1 && (segs[0] == "." || segs[0] == "..") {
		if segs[0] == ".." {
			ups++
		}
		navigated = true
		segs = segs[1:]
	}
	citerSegs := dirSegs(citerDir)
	if ups > len(citerSegs) {
		return "", false
	}
	want := strings.Split(target.path, "/")

	if navigated {
		resolved := append(append([]string{}, citerSegs[:len(citerSegs)-ups]...), segs...)
		if !equalSegs(resolved, want) {
			return "", false
		}
		return MatchPath, true
	}

	if len(segs) > len(want) {
		return "", false
	}
	for k := 1; k <= len(segs); k++ {
		if segs[len(segs)-k] != want[len(want)-k] {
			return "", false
		}
	}
	// A bare single segment is checked for extension regardless of which
	// branch below it would otherwise take — including the exact-match one,
	// which a root-level extensionless target reaches on a bare mention just
	// as easily as a suffix match does.
	if len(segs) == 1 && !strings.Contains(target.base, ".") {
		return "", false
	}
	if len(segs) == len(want) {
		return MatchPath, true
	}
	if target.baseShared {
		return "", false
	}
	return MatchBasename, true
}

// findReference scans line for a mention of target, returning the offset of the
// reference as written, how strong the match is, and its text — or -1 when the
// line does not name target.
//
// Every path ends in its own basename, so one scan for the basename finds every
// occurrence worth considering; the widened token around each is what decides.
// A rejected occurrence does not end the scan, so a line carrying both
// "archive/references/data.csv" and "references/data.csv" still cites the
// second.
func findReference(line string, target candidate, citerDir string) (int, MatchKind, string) {
	for from := 0; ; {
		i := strings.Index(line[from:], target.base)
		if i < 0 {
			return -1, "", ""
		}
		i += from
		from = i + 1

		start, end := pathTokenAround(line, i, len(target.base))
		written := line[start:end]
		if kind, ok := classifyReference(written, target, citerDir); ok {
			return start, kind, written
		}
	}
}

// dirSegs splits a forward-slashed directory into its path segments, treating
// "" (the workspace root) as zero segments — the shape classifyReference needs
// to resolve navigation by stripping segments off the end.
func dirSegs(dir string) []string {
	if dir == "" {
		return nil
	}
	return strings.Split(dir, "/")
}

func equalSegs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// citerDirOf returns rel's directory, forward-slashed, with the workspace
// root normalized to "" — path.Dir returns "." for a root-level file, which
// dirSegs would otherwise have to special-case a second time.
func citerDirOf(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		return ""
	}
	return dir
}

type candidate struct {
	path string // full relative path, forward-slashed
	base string // bare filename, always populated
	// baseShared marks a basename that collides with another file's, so
	// bare-name matching is suppressed for it and only a full-path mention
	// can cite it.
	baseShared bool
}

// buildCandidates returns one candidate per relative path, in the same
// (sorted) order Discover already computed relPaths in; crossReference walks
// them in that order, so a line's citations come out sorted by ToPath.
//
// baseShared marks the basenames that are not unique across the tree. On a real
// workspace, generic names like "config.json" or dated output like
// "workspace/2024-01-01/report.json" can share a basename with dozens of
// unrelated files, and matching all of them turns one real citation into a
// flood of false ones. Such a file is still citable by its full relative path,
// which is unambiguous — just not by any shorter reference.
func buildCandidates(relPaths []string) []candidate {
	baseCounts := make(map[string]int, len(relPaths))
	for _, rel := range relPaths {
		baseCounts[path.Base(rel)]++
	}

	out := make([]candidate, 0, len(relPaths))
	for _, rel := range relPaths {
		base := path.Base(rel)
		out = append(out, candidate{path: rel, base: base, baseShared: baseCounts[base] > 1})
	}
	return out
}

func readLines(fullPath string) ([]string, error) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

// snippetOf returns a bounded, human/LLM-readable preview of line for use as
// Citation.Snippet, guaranteed to contain the match spanning
// [matchStart, matchStart+matchLen) as long as the match itself is shorter
// than maxSnippetLen — true of any realistic path. That containment is the
// whole point of Snippet: it exists so a consumer can judge a citation
// without re-opening FromPath, and a preview that doesn't actually show the
// match fails that job silently while still looking like a valid citation.
// Unconditionally taking a line's first maxSnippetLen bytes (an earlier
// version of this function did) broke that guarantee on any line longer than
// the cutoff where the match fell past it.
//
// If line fits within maxSnippetLen entirely, it is returned whole, trimmed —
// the cheap, common case, and the only one before this function took a match
// position. Otherwise a maxSnippetLen-byte window is centered on the match's
// midpoint and clamped to the line's bounds.
//
// The clone is load-bearing. readLines splits one string covering the whole
// file, so trimming and slicing only re-slice: without copying, every
// snippet keeps its entire source file alive for as long as the Inventory
// lives, and retention scales with total markdown volume rather than with the
// number of citations. Measured on ten 500 KB files with one citation each,
// 520 bytes of snippet text held 5 MB of heap. Citation is documented as
// cheap to carry into a prompt, and that has to be true of its backing store
// too, not just the struct.
func snippetOf(line string, matchStart, matchLen int) string {
	if len(line) <= maxSnippetLen {
		return strings.Clone(strings.TrimSpace(line))
	}
	mid := matchStart + matchLen/2
	start := mid - maxSnippetLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxSnippetLen
	if end > len(line) {
		end = len(line)
		start = end - maxSnippetLen
		if start < 0 {
			start = 0
		}
	}
	return strings.Clone(strings.TrimSpace(line[start:end]))
}
