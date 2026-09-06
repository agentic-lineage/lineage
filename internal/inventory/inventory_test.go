package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// buildMixedWorkspace lays out a fixture covering the realistic messy-
// package shapes #102 targets: root CLAUDE.md/AGENTS.md, a skill with a
// colocated helper script it names, a workflow that names both that skill's
// script and a standalone script (the fan-in case — one script cited by two
// different instruction files), a reference file, a setup note, an
// unreferenced orphan script, and ignored-by-default directories.
func buildMixedWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Project instructions\n\nGeneral guidance for the assistant.\n")
	mustWrite(t, filepath.Join(root, "AGENTS.md"), "# Codex instructions\n\nSame general guidance, mirrored for Codex.\n")
	mustWrite(t, filepath.Join(root, "README.md"), "# Setup\n\nRun the install steps below.\n")

	mustWrite(t, filepath.Join(root, "skills", "foo", "SKILL.md"),
		"# Foo skill\n\nInvoke the helper by running `run.sh` after setup.\n")
	mustWrite(t, filepath.Join(root, "skills", "foo", "run.sh"), "#!/bin/sh\necho foo\n")

	mustWrite(t, filepath.Join(root, "workflows", "deploy", "WORKFLOW.md"),
		"# Deploy workflow\n\n1. Run skills/foo/run.sh to prepare.\n2. Run scripts/deploy.sh to ship.\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")
	mustWrite(t, filepath.Join(root, "scripts", "orphan.sh"), "#!/bin/sh\necho orphan\n")

	mustWrite(t, filepath.Join(root, "references", "notes.md"), "# Notes\n\nBackground reference material.\n")

	mustWrite(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	mustWrite(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "module.exports = {};\n")

	return root
}

func TestDiscoverDeterministic(t *testing.T) {
	root := buildMixedWorkspace(t)

	first, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	second, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() second run error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Discover() not deterministic:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestDiscoverSkipsIgnoredDirs(t *testing.T) {
	root := buildMixedWorkspace(t)

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	for _, e := range inv.Entries {
		if hasIgnoredSegment(e.Path) {
			t.Fatalf("entry %q should have been skipped as ignored", e.Path)
		}
	}
}

func hasIgnoredSegment(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if isIgnoredDir(seg) {
			return true
		}
	}
	return false
}

func TestDiscoverClassifiesRealisticShapes(t *testing.T) {
	root := buildMixedWorkspace(t)

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	byPath := entryMap(inv)
	cases := []struct {
		path string
		kind ArtifactKind
	}{
		{"CLAUDE.md", KindInstruction},
		{"AGENTS.md", KindInstruction},
		{"README.md", KindSetupMaterial},
		{"skills/foo/SKILL.md", KindInstruction},
		{"skills/foo/run.sh", KindExecutableHelper},
		{"workflows/deploy/WORKFLOW.md", KindInstruction},
		{"scripts/deploy.sh", KindExecutableHelper},
		{"scripts/orphan.sh", KindExecutableHelper},
		{"references/notes.md", KindReference},
	}
	for _, c := range cases {
		e, ok := byPath[c.path]
		if !ok {
			t.Fatalf("missing expected entry %q", c.path)
		}
		if e.Kind != c.kind {
			t.Errorf("%s: Kind = %q, want %q (reason: %s)", c.path, e.Kind, c.kind, e.Reason)
		}
		if e.Digest == "" {
			t.Errorf("%s: Digest is empty", c.path)
		}
		if e.Reason == "" {
			t.Errorf("%s: Reason is empty", c.path)
		}
	}
}

func TestDiscoverCitationSingleReference(t *testing.T) {
	root := buildMixedWorkspace(t)

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	wf := byPath["workflows/deploy/WORKFLOW.md"]
	deploy := byPath["scripts/deploy.sh"]

	if len(deploy.ReferencedBy) != 1 {
		t.Fatalf("scripts/deploy.sh ReferencedBy = %+v, want exactly 1 citation", deploy.ReferencedBy)
	}
	want := pathCite(t, "workflows/deploy/WORKFLOW.md", "scripts/deploy.sh", 4, "2. Run scripts/deploy.sh to ship.")
	if deploy.ReferencedBy[0] != want {
		t.Errorf("scripts/deploy.sh ReferencedBy[0] = %+v, want %+v", deploy.ReferencedBy[0], want)
	}
	if !containsCitation(wf.Mentions, want) {
		t.Errorf("workflows/deploy/WORKFLOW.md Mentions = %+v, want to contain %+v", wf.Mentions, want)
	}

	// Reason must never be overwritten by the citation pass.
	if deploy.Reason == "" || deploy.Reason == want.Snippet {
		t.Errorf("scripts/deploy.sh Reason was corrupted by citation pass: %q", deploy.Reason)
	}
}

func TestDiscoverCitationFanIn(t *testing.T) {
	root := buildMixedWorkspace(t)

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	runSh := byPath["skills/foo/run.sh"]
	if len(runSh.ReferencedBy) != 2 {
		t.Fatalf("skills/foo/run.sh ReferencedBy = %+v, want exactly 2 citations (SKILL.md and WORKFLOW.md)", runSh.ReferencedBy)
	}

	froms := make([]string, len(runSh.ReferencedBy))
	for i, c := range runSh.ReferencedBy {
		froms[i] = c.FromPath
	}
	sort.Strings(froms)
	want := []string{"skills/foo/SKILL.md", "workflows/deploy/WORKFLOW.md"}
	if !reflect.DeepEqual(froms, want) {
		t.Errorf("skills/foo/run.sh ReferencedBy froms = %v, want %v", froms, want)
	}
}

func TestDiscoverOrphanScriptUnreferenced(t *testing.T) {
	root := buildMixedWorkspace(t)

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	orphan := byPath["scripts/orphan.sh"]
	if len(orphan.ReferencedBy) != 0 {
		t.Fatalf("scripts/orphan.sh ReferencedBy = %+v, want empty (truly unreferenced)", orphan.ReferencedBy)
	}
	if orphan.Reason == "" {
		t.Error("scripts/orphan.sh Reason must still be set even though unreferenced")
	}
}

// TestDiscoverAmbiguousBasenameOnlyMatchesFullPath reproduces a real bug
// found by running Discover against an actual messy repo: a workflow
// referencing a dated output file by its bare name (e.g. "report.json")
// must not fan out to every unrelated file sharing that basename elsewhere
// in the tree. Only an unambiguous full-path mention should cite.
func TestDiscoverAmbiguousBasenameOnlyMatchesFullPath(t *testing.T) {
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, "workspace", "2024-01-01", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "workspace", "2024-01-02", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "workspace", "2024-01-03", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"),
		"# Debugging\n\nIf `report.json` is missing, check the fetch step.\n\n"+
			"Full path: workspace/2024-01-02/report.json is the one that matters here.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	for _, day := range []string{"2024-01-01", "2024-01-03"} {
		p := "workspace/" + day + "/report.json"
		e := byPath[p]
		if len(e.ReferencedBy) != 0 {
			t.Errorf("%s ReferencedBy = %+v, want empty (bare-name match is ambiguous, must not fan out)", p, e.ReferencedBy)
		}
	}

	cited := byPath["workspace/2024-01-02/report.json"]
	if len(cited.ReferencedBy) != 1 {
		t.Fatalf("workspace/2024-01-02/report.json ReferencedBy = %+v, want exactly 1 (unambiguous full-path mention)", cited.ReferencedBy)
	}
	if cited.ReferencedBy[0].Line != 5 {
		t.Errorf("ReferencedBy[0].Line = %d, want 5", cited.ReferencedBy[0].Line)
	}
}

// TestDiscoverMentionsIdentifyTargets is the regression for the original
// API-contract gap: an outgoing Mentions entry recorded only FromPath — the
// citing file itself — so a consumer could not tell what had been named, and
// two targets on one line produced byte-identical citations.
func TestDiscoverMentionsIdentifyTargets(t *testing.T) {
	root := t.TempDir()
	const line = "Run scripts/deploy.sh then scripts/verify.sh to ship."
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Release\n\n"+line+"\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "scripts", "verify.sh"), "#!/bin/sh\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	got := byPath["CLAUDE.md"].Mentions
	want := []Citation{
		pathCite(t, "CLAUDE.md", "scripts/deploy.sh", 3, line),
		pathCite(t, "CLAUDE.md", "scripts/verify.sh", 3, line),
	}
	// Both edges share a line and a snippet; ToPath and Column are what make
	// them distinguishable, and the order is ToPath-sorted.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CLAUDE.md Mentions = %+v, want %+v", got, want)
	}
}

// TestDiscoverCitationReportsMatchStrength asserts a bare-name citation is
// labelled as the weaker evidence it is, so a consumer can weigh a filename
// that merely happens to be unique differently from an explicit path.
func TestDiscoverCitationReportsMatchStrength(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "skills", "foo", "SKILL.md"),
		"# Foo\n\nInvoke the helper by running `run.sh` after setup.\n")
	mustWrite(t, filepath.Join(root, "skills", "foo", "run.sh"), "#!/bin/sh\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	got := byPath["skills/foo/run.sh"].ReferencedBy
	if len(got) != 1 {
		t.Fatalf("skills/foo/run.sh ReferencedBy = %+v, want exactly 1", got)
	}
	if got[0].MatchKind != MatchBasename {
		t.Errorf("MatchKind = %q, want %q (named as a bare filename, not a path)", got[0].MatchKind, MatchBasename)
	}
	if got[0].AsWritten != "run.sh" {
		t.Errorf("AsWritten = %q, want %q", got[0].AsWritten, "run.sh")
	}
	if got[0].ToPath != "skills/foo/run.sh" {
		t.Errorf("ToPath = %q, want %q", got[0].ToPath, "skills/foo/run.sh")
	}
}

// TestDiscoverFlagsAmbiguousBasename covers the distinction a consumer needs
// in order to read an empty ReferencedBy correctly: a file whose bare-name
// matching was suppressed is not the same as a genuinely unreferenced one,
// and concluding "unused" from the former would be wrong.
func TestDiscoverFlagsAmbiguousBasename(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "b", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "scripts", "orphan.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nCheck `report.json` when debugging.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	for _, p := range []string{"a/report.json", "b/report.json"} {
		e := byPath[p]
		if !e.AmbiguousBasename {
			t.Errorf("%s AmbiguousBasename = false, want true (basename shared)", p)
		}
		if len(e.ReferencedBy) != 0 {
			t.Errorf("%s ReferencedBy = %+v, want empty (bare name is ambiguous)", p, e.ReferencedBy)
		}
	}
	// The orphan is equally unreferenced, but for the other reason — and is
	// distinguishable precisely because the flag is not set.
	orphan := byPath["scripts/orphan.sh"]
	if orphan.AmbiguousBasename {
		t.Error("scripts/orphan.sh AmbiguousBasename = true, want false (basename is unique)")
	}
	if len(orphan.ReferencedBy) != 0 {
		t.Errorf("scripts/orphan.sh ReferencedBy = %+v, want empty", orphan.ReferencedBy)
	}
}

func TestDiscoverStampsSchema(t *testing.T) {
	inv, err := Discover(buildMixedWorkspace(t))
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if inv.Schema != CurrentSchema {
		t.Errorf("Schema = %d, want %d", inv.Schema, CurrentSchema)
	}
}

// TestDiscoverCitationRequiresTokenBoundary pins the substring false
// positives that plain strings.Contains matching produced: a line naming a
// backup or a similarly-prefixed directory must not cite the shorter path it
// happens to contain. Citations are evidence, so a match that is really part
// of a longer name is worse than no match at all.
func TestDiscoverCitationRequiresTokenBoundary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\necho deploy\n")
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh.bak"), "#!/bin/sh\necho old\n")
	mustWrite(t, filepath.Join(root, "myscripts", "deploy.sh"), "#!/bin/sh\necho other\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"),
		"# Notes\n\nThe old scripts/deploy.sh.bak is dead, ignore it.\n"+
			"Nothing here runs myscripts/deploy.sh either.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	// Suffix case: "scripts/deploy.sh.bak" contains "scripts/deploy.sh".
	if got := byPath["scripts/deploy.sh"].ReferencedBy; len(got) != 0 {
		t.Errorf("scripts/deploy.sh ReferencedBy = %+v, want empty (only the .bak file was named)", got)
	}
	// Prefix case: "myscripts/deploy.sh" contains "scripts/deploy.sh".
	if got := byPath["scripts/deploy.sh.bak"].ReferencedBy; len(got) != 1 {
		t.Errorf("scripts/deploy.sh.bak ReferencedBy = %+v, want exactly 1 (it is the file actually named)", got)
	}
	if got := byPath["myscripts/deploy.sh"].ReferencedBy; len(got) != 1 {
		t.Errorf("myscripts/deploy.sh ReferencedBy = %+v, want exactly 1", got)
	}
}

// TestDiscoverCitationSurvivesAdjacentPunctuation guards the other side of
// the boundary rule: tightening the matcher must not start dropping real
// citations that sit next to ordinary prose punctuation, or next to a
// relative path prefix.
func TestDiscoverCitationSurvivesAdjacentPunctuation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "scripts", "build.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "references", "data.csv"), "a,b\n")
	mustWrite(t, filepath.Join(root, "skills", "foo", "SKILL.md"),
		"Run `build.sh`, then read ../../references/data.csv.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	// Backtick-wrapped bare name, and a path reached through "../../" whose
	// sentence-ending period must not read as a filename extension.
	if got := byPath["scripts/build.sh"].ReferencedBy; len(got) != 1 {
		t.Errorf("scripts/build.sh ReferencedBy = %+v, want 1 (backtick-wrapped bare name)", got)
	}
	got := byPath["references/data.csv"].ReferencedBy
	if len(got) != 1 {
		t.Fatalf("references/data.csv ReferencedBy = %+v, want 1 (relative prefix + trailing period)", got)
	}
	// AsWritten reports the reference as the prose wrote it, relative prefix
	// included — that prefix is what a consumer needs to resolve it, and it is
	// the reason AsWritten is not simply ToPath.
	if want := "../../references/data.csv"; got[0].AsWritten != want {
		t.Errorf("AsWritten = %q, want %q", got[0].AsWritten, want)
	}
	line := "Run `build.sh`, then read ../../references/data.csv."
	if want := strings.Index(line, "../../") + 1; got[0].Column != want {
		t.Errorf("Column = %d, want %d (start of the written reference)", got[0].Column, want)
	}
}

// TestDiscoverRejectsLongerPathEndingInTarget is the reviewer-reported case:
// a directory prefix means the prose named a *different* file that merely
// shares a tail. Round 1 accepted any "/" before a match, which made this a
// citation.
func TestDiscoverRejectsLongerPathEndingInTarget(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "references", "data.csv"), "a,b\n")
	mustWrite(t, filepath.Join(root, "archive", "references", "data.csv"), "old,data\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"),
		"# Notes\n\nOnly archive/references/data.csv is stale.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	if got := byPath["references/data.csv"].ReferencedBy; len(got) != 0 {
		t.Errorf("references/data.csv ReferencedBy = %+v, want empty (only the archive copy was named)", got)
	}
	got := byPath["archive/references/data.csv"].ReferencedBy
	if len(got) != 1 {
		t.Fatalf("archive/references/data.csv ReferencedBy = %+v, want exactly 1", got)
	}
	if got[0].MatchKind != MatchPath {
		t.Errorf("MatchKind = %q, want %q", got[0].MatchKind, MatchPath)
	}
}

// TestDiscoverRejectsSubpathOfTarget is the same defect at the other boundary,
// found by inspecting for it rather than reported: an entry with no extension
// is a prefix of a longer path, and round 1 accepted a "/" immediately after a
// match just as it did before one.
func TestDiscoverRejectsSubpathOfTarget(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "scripts", "build"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"),
		"# Notes\n\nArtifacts land in scripts/build/output.txt after a run.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	if got := byPath["scripts/build"].ReferencedBy; len(got) != 0 {
		t.Errorf("scripts/build ReferencedBy = %+v, want empty (a path under it was named, not it)", got)
	}
}

// TestDiscoverCitesPartialPath pins the behavior the reviewer's literal
// suggestion (allow only "./" and "../") would have dropped: naming a file by
// a partial path is a real reference, and messy workspaces write them.
func TestDiscoverCitesPartialPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "skills", "foo", "run.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nInvoke foo/run.sh to start.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	got := byPath["skills/foo/run.sh"].ReferencedBy
	if len(got) != 1 {
		t.Fatalf("skills/foo/run.sh ReferencedBy = %+v, want exactly 1", got)
	}
	// A suffix of the path, so weaker than an exact mention but real.
	if got[0].MatchKind != MatchBasename {
		t.Errorf("MatchKind = %q, want %q", got[0].MatchKind, MatchBasename)
	}
	if want := "foo/run.sh"; got[0].AsWritten != want {
		t.Errorf("AsWritten = %q, want %q", got[0].AsWritten, want)
	}
}

// TestDiscoverSuppressesAmbiguousPartialPath extends the bare-name ambiguity
// guard to partial paths: "y/report.json" identifies a file only if the
// basename is unique, and here it is not.
func TestDiscoverSuppressesAmbiguousPartialPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "x", "y", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "z", "y", "report.json"), "{}\n")
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nCheck y/report.json when debugging.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	for _, p := range []string{"x/y/report.json", "z/y/report.json"} {
		e := byPath[p]
		if len(e.ReferencedBy) != 0 {
			t.Errorf("%s ReferencedBy = %+v, want empty (the partial path names both)", p, e.ReferencedBy)
		}
		if !e.AmbiguousBasename {
			t.Errorf("%s AmbiguousBasename = false, want true", p)
		}
	}
}

// TestDiscoverHandlesNonASCIIPathSegments covers both directions of the
// byte-class gap. The matcher works on bytes, and every byte of a multi-byte
// UTF-8 sequence is >= 0x80: read as a separator, an accented letter both
// invents citations and — once widening decides matches — truncates a real
// path at its non-ASCII segment and loses one.
func TestDiscoverHandlesNonASCIIPathSegments(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "café", "data.csv"), "a,b\n")
	mustWrite(t, filepath.Join(root, "notas.md"), "# Notas\n\nUnrelated.\n")
	// "añnotas.md" puts the ñ's trailing byte directly against the match, which
	// is what it takes to reproduce the false positive — an ASCII letter in
	// between (as in "añonotas.md") is already rejected as a token byte.
	mustWrite(t, filepath.Join(root, "CLAUDE.md"),
		"# Notes\n\nRead docs/café/data.csv first.\nSee añnotas.md for the rest.\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	// Real citation must survive the accented directory segment.
	got := byPath["docs/café/data.csv"].ReferencedBy
	if len(got) != 1 {
		t.Fatalf("docs/café/data.csv ReferencedBy = %+v, want exactly 1", got)
	}
	if want := "docs/café/data.csv"; got[0].AsWritten != want {
		t.Errorf("AsWritten = %q, want %q (must not truncate at the non-ASCII segment)", got[0].AsWritten, want)
	}
	if got[0].MatchKind != MatchPath {
		t.Errorf("MatchKind = %q, want %q", got[0].MatchKind, MatchPath)
	}

	// And an accent must not read as a word break: "añonotas.md" is not a
	// mention of "notas.md".
	if got := byPath["notas.md"].ReferencedBy; len(got) != 0 {
		t.Errorf("notas.md ReferencedBy = %+v, want empty (a different filename was named)", got)
	}
}

// TestDiscoverCitationPathShapes sweeps the path forms real prose actually
// contains — absolute, relative, navigated, URL, Windows-separated, and paths
// naming files that do not exist — and pins what each does.
//
// The safety-relevant rows are the navigated ones. Citations are pure string
// matching over a read-only walk, so nothing here can reach outside the
// workspace on disk; what it CAN do is manufacture evidence about a file
// inside the tree from a reference that points outside it, which is what the
// escape rows guard.
func TestDiscoverCitationPathShapes(t *testing.T) {
	cases := []struct {
		name    string
		citer   string // markdown file holding the mention
		mention string // the reference exactly as written in prose
		target  string // inventoried file we assert about
		want    MatchKind
		cited   bool
	}{
		// Forms that name the target and should cite.
		{"exact path", "CLAUDE.md", "See references/data.csv here.", "references/data.csv", MatchPath, true},
		{"explicit ./", "CLAUDE.md", "See ./references/data.csv here.", "references/data.csv", MatchPath, true},
		{"navigation within depth", "skills/foo/SKILL.md", "See ../../references/data.csv here.", "references/data.csv", MatchPath, true},
		{"bare unique basename", "CLAUDE.md", "See data.csv here.", "references/data.csv", MatchBasename, true},
		{"partial path", "CLAUDE.md", "See foo/run.sh here.", "skills/foo/run.sh", MatchBasename, true},
		{"hidden file", "CLAUDE.md", "Copy .env before running.", ".env", MatchPath, true},
		// A query string is not part of the path token, so the path still reads.
		{"trailing query string", "CLAUDE.md", "See references/data.csv?v=2 here.", "references/data.csv", MatchPath, true},
		// Windows separator: the backslash ends the token, leaving a bare name,
		// which is honestly reported as the weaker basename evidence.
		{"windows separator", "CLAUDE.md", `See scripts\deploy.sh here.`, "scripts/deploy.sh", MatchBasename, true},

		// Escapes: the reference climbs above the workspace root, so it names
		// something outside the tree and is not evidence about anything in it.
		{"navigation past root", "CLAUDE.md", "See ../../references/data.csv here.", "references/data.csv", "", false},
		{"deep escape to system path", "CLAUDE.md", "Unlike ../../../../../../etc/data.csv, ours is local.", "etc/data.csv", "", false},
		{"escape from shallow citer", "docs/guide.md", "See ../../references/data.csv here.", "references/data.csv", "", false},

		// Forms that name a different file, or no file in this tree.
		{"absolute path", "CLAUDE.md", "See /references/data.csv here.", "references/data.csv", "", false},
		{"url", "CLAUDE.md", "See https://example.com/references/data.csv here.", "references/data.csv", "", false},
		{"home-relative", "CLAUDE.md", "See ~/data.csv here.", "references/data.csv", "", false},
		{"different directory", "CLAUDE.md", "See other/data.csv here.", "references/data.csv", "", false},
		{"longer path ending in target", "CLAUDE.md", "See archive/references/data.csv here.", "references/data.csv", "", false},
		{"path below target", "CLAUDE.md", "See references/data.csv/old here.", "references/data.csv", "", false},
		{"case mismatch", "CLAUDE.md", "See References/Data.csv here.", "references/data.csv", "", false},
		{"empty segment", "CLAUDE.md", "See references//data.csv here.", "references/data.csv", "", false},
		{"unnormalized mid-path dot", "CLAUDE.md", "See references/./data.csv here.", "references/data.csv", "", false},
		{"sibling that does not exist", "CLAUDE.md", "See references/missing.csv here.", "references/data.csv", "", false},
		{"parent then back down", "CLAUDE.md", "See ../repo/references/data.csv here.", "references/data.csv", "", false},

		// Partial unwind: climbs some but not all the way to the root, so it
		// resolves at an intermediate ancestor directory rather than either
		// the true root or the citer's own directory. Round 3's ups-vs-depth
		// bound alone accepted this (1 <= depth 2) without checking WHERE it
		// actually lands, so "../scripts/deploy.sh" from skills/foo/SKILL.md
		// cited a same-named root-level file instead of the real target,
		// skills/scripts/deploy.sh — the wrong-directory case round 1's
		// "within depth" row (which always lands exactly at the root) could
		// never expose.
		{"navigation lands in wrong ancestor", "skills/foo/SKILL.md", "See ../scripts/deploy.sh here.", "scripts/deploy.sh", "", false},
		{"navigation lands in correct ancestor", "skills/foo/SKILL.md", "See ../scripts/deploy.sh here.", "skills/scripts/deploy.sh", MatchPath, true},

		// "./" from a nested citer resolves relative to that citer's own
		// directory, not fuzzy-matched against the whole tree: it must find
		// the file actually beside the citer, and nowhere else with the same
		// bare name.
		{"same-directory dot resolves against nested citer", "skills/foo/SKILL.md", "See ./run.sh here.", "skills/foo/run.sh", MatchPath, true},
		{"same-directory dot does not reach root", "skills/foo/SKILL.md", "See ./run.sh here.", "run.sh", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, filepath.Join(root, filepath.FromSlash(c.target)), "content\n")
			mustWrite(t, filepath.Join(root, filepath.FromSlash(c.citer)), "# Doc\n\n"+c.mention+"\n")

			inv, err := Discover(root)
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			got := entryMap(inv)[c.target].ReferencedBy

			if !c.cited {
				if len(got) != 0 {
					t.Fatalf("%s ReferencedBy = %+v, want empty", c.target, got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("%s ReferencedBy = %+v, want exactly 1", c.target, got)
			}
			if got[0].MatchKind != c.want {
				t.Errorf("MatchKind = %q, want %q (matched %q)", got[0].MatchKind, c.want, got[0].AsWritten)
			}
		})
	}
}

// TestDiscoverRejectsUnusableRoot covers the remaining preconditions on the
// discovery root beside the symlink case: a root that is missing, or is a
// regular file, must fail loudly rather than walk nothing and report a
// successful empty inventory.
func TestDiscoverRejectsUnusableRoot(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		inv, err := Discover(filepath.Join(t.TempDir(), "no-such-workspace"))
		if err == nil {
			t.Fatalf("Discover() error = nil with %d entries, want an error", len(inv.Entries))
		}
	})

	t.Run("regular file", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "CLAUDE.md")
		mustWrite(t, file, "# Not a workspace\n")

		inv, err := Discover(file)
		if err == nil {
			t.Fatalf("Discover() error = nil with %d entries, want an error", len(inv.Entries))
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Errorf("Discover(file) error = %v, want it to say the root is not a directory", err)
		}
	})
}

// TestDiscoverIgnoresBareExtensionlessWordsInProse pins the fix for a common
// false positive round 3's rewrite left standing: a workspace file whose name
// has no extension — "run", "build", "test", "main" are all realistic
// contents of a bin/ or Makefile-target directory — is indistinguishable from
// the same word appearing in ordinary sentences once path separators are
// stripped. "Please run the tests" must not cite a file literally named
// "run", whether that file sits at the workspace root (an exact-path match by
// construction) or nested (a basename match) — extension is what makes a
// bare mention safe to trust, not location.
func TestDiscoverIgnoresBareExtensionlessWordsInProse(t *testing.T) {
	t.Run("extensionless root file not cited by prose", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "run"), "#!/bin/sh\n")
		mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nPlease run the tests before committing.\n")

		inv, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}
		if got := entryMap(inv)["run"].ReferencedBy; len(got) != 0 {
			t.Errorf("run ReferencedBy = %+v, want empty (the word \"run\" in prose is not a citation)", got)
		}
	})

	t.Run("extensionless nested file not cited by prose", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "bin", "run"), "#!/bin/sh\n")
		mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nPlease run the tests before committing.\n")

		inv, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}
		if got := entryMap(inv)["bin/run"].ReferencedBy; len(got) != 0 {
			t.Errorf("bin/run ReferencedBy = %+v, want empty", got)
		}
	})

	t.Run("extensioned root file still cited by prose", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "run.sh"), "#!/bin/sh\n")
		mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nPlease run.sh before committing.\n")

		inv, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}
		got := entryMap(inv)["run.sh"].ReferencedBy
		if len(got) != 1 {
			t.Fatalf("run.sh ReferencedBy = %+v, want exactly 1", got)
		}
		if got[0].MatchKind != MatchPath {
			t.Errorf("MatchKind = %q, want %q", got[0].MatchKind, MatchPath)
		}
	})

	t.Run("multi-segment path to extensionless file still cites", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "bin", "run"), "#!/bin/sh\n")
		mustWrite(t, filepath.Join(root, "CLAUDE.md"), "# Notes\n\nInvoke bin/run before committing.\n")

		inv, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}
		got := entryMap(inv)["bin/run"].ReferencedBy
		if len(got) != 1 {
			t.Fatalf("bin/run ReferencedBy = %+v, want exactly 1 (a slash is never ambiguous with prose)", got)
		}
		if got[0].MatchKind != MatchPath {
			t.Errorf("MatchKind = %q, want %q", got[0].MatchKind, MatchPath)
		}
	})
}

// TestDiscoverSnippetAlwaysContainsMatch pins the fix for Snippet silently
// failing its one job on a long line: it used to always take a line's first
// maxSnippetLen bytes, so a match past that offset produced a Snippet showing
// unrelated earlier text while Column/AsWritten correctly pointed elsewhere —
// a citation that looks justified but isn't, to any consumer (especially an
// LLM) reading Snippet as the evidence for why the edge exists.
func TestDiscoverSnippetAlwaysContainsMatch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "scripts", "deploy.sh"), "#!/bin/sh\n")

	filler := strings.Repeat("x", 250)
	line := "# " + filler + " then run scripts/deploy.sh to ship."
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), line+"\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	got := entryMap(inv)["scripts/deploy.sh"].ReferencedBy
	if len(got) != 1 {
		t.Fatalf("scripts/deploy.sh ReferencedBy = %+v, want exactly 1", got)
	}
	c := got[0]
	if len(line) <= maxSnippetLen {
		t.Fatalf("test fixture line is %d bytes, want > %d to exercise truncation", len(line), maxSnippetLen)
	}
	if !strings.Contains(c.Snippet, c.AsWritten) {
		t.Errorf("Snippet = %q does not contain AsWritten = %q (match offset %d, line length %d)",
			c.Snippet, c.AsWritten, c.Column, len(line))
	}
}

// TestDiscoverRecognizesProviderPrefixedAgentFiles reproduces a real gap
// found by running Discover against an actual public Claude Code project
// (ChrisWiles/claude-code-showcase): .claude/agents/code-reviewer.md is
// genuine instruction content (YAML frontmatter + prose, structurally
// identical to a SKILL.md) but was classified unknown, because the
// directory-based instruction rule only recognized bare top-level
// skills/workflows/, not the .claude/-prefixed convention real Claude Code
// repos actually use, and had no notion of an "agents" directory at all.
func TestDiscoverRecognizesProviderPrefixedAgentFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".claude", "agents", "code-reviewer.md"),
		"---\nname: code-reviewer\n---\n\nSenior code reviewer. Run `git diff` and review changes.\n")
	// Exercises the ".agents" provider-root branch symmetrically — not
	// itself an observed real-world path, just coverage for the second
	// known prefix alongside the ".claude" one found in the wild above.
	mustWrite(t, filepath.Join(root, ".agents", "agents", "deployer.md"),
		"Deploys the app after tests pass.\n")
	// A flat file directly in "agents/" at the bare top level (no provider
	// prefix) must also match, mirroring how Lineage's own agents/ content
	// dir works (flat files, per internal/packages).
	mustWrite(t, filepath.Join(root, "agents", "reviewer.md"), "Reviews things.\n")
	// One level deeper than a provider root's agents/ dir is auxiliary
	// material, not the agent's own file, and must NOT match.
	mustWrite(t, filepath.Join(root, ".claude", "agents", "notes", "scratch.md"), "unrelated notes\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	for _, p := range []string{
		".claude/agents/code-reviewer.md",
		".agents/agents/deployer.md",
		"agents/reviewer.md",
	} {
		if e, ok := byPath[p]; !ok || e.Kind != KindInstruction {
			t.Errorf("%s Kind = %v, want %q", p, e.Kind, KindInstruction)
		}
	}
	if e := byPath[".claude/agents/notes/scratch.md"]; e.Kind == KindInstruction {
		t.Errorf(".claude/agents/notes/scratch.md Kind = %q, want anything but %q (one level too deep)", e.Kind, KindInstruction)
	}
}

func TestDiscoverUnknownIsHonestNotGuessed(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "mystery.xyz"), "no rule matches this\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)
	e, ok := byPath["mystery.xyz"]
	if !ok {
		t.Fatal("missing mystery.xyz entry")
	}
	if e.Kind != KindUnknown {
		t.Errorf("Kind = %q, want %q", e.Kind, KindUnknown)
	}
	if e.Reason != "no classification rule matched" {
		t.Errorf("Reason = %q, want honest no-match reason", e.Reason)
	}
}

func TestDiscoverRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "real.txt"), "content\n")

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	if _, err := Discover(root); err == nil {
		t.Fatal("Discover() error = nil, want error for symlinked content")
	}
}

// TestDiscoverRejectsSymlinkRoot covers the root argument itself, which the
// in-tree check above never sees: the walk callback returns early on the root
// entry, before reaching its symlink check, so a symlinked root previously
// produced a successful empty inventory rather than an error. Silently
// reporting "nothing here" for a workspace that was never scanned is the
// failure mode this guards.
func TestDiscoverRejectsSymlinkRoot(t *testing.T) {
	real := t.TempDir()
	mustWrite(t, filepath.Join(real, "CLAUDE.md"), "# Instructions\n")

	link := filepath.Join(t.TempDir(), "link-to-workspace")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	inv, err := Discover(link)
	if err == nil {
		t.Fatalf("Discover(symlink root) error = nil with %d entries, want an error", len(inv.Entries))
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Discover(symlink root) error = %v, want it to name the symlink", err)
	}

	// The real path still works, so this refuses the indirection, not the
	// workspace behind it.
	if _, err := Discover(real); err != nil {
		t.Errorf("Discover(real root) error = %v, want success", err)
	}
}

func entryMap(inv Inventory) map[string]Entry {
	m := make(map[string]Entry, len(inv.Entries))
	for _, e := range inv.Entries {
		m[e.Path] = e
	}
	return m
}

// pathCite builds the expected full-path citation for a line, deriving
// Column and Snippet from the source text so a seven-field expectation stays
// readable at the call site.
func pathCite(t *testing.T, from, to string, line int, text string) Citation {
	t.Helper()
	col := strings.Index(text, to)
	if col < 0 {
		t.Fatalf("pathCite: %q does not appear in %q", to, text)
	}
	return Citation{
		FromPath:  from,
		ToPath:    to,
		Line:      line,
		Column:    col + 1,
		MatchKind: MatchPath,
		AsWritten: to,
		Snippet:   strings.TrimSpace(text),
	}
}

func containsCitation(cs []Citation, want Citation) bool {
	for _, c := range cs {
		if c == want {
			return true
		}
	}
	return false
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
