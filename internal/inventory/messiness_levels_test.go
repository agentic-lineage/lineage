package inventory

import (
	"path/filepath"
	"testing"
)

// These five fixtures are the same semantic workflow as
// examples/resume-workflow (gather notes, then review), rewritten at
// decreasing levels of ambiguity — the shapes a real user's workspace
// might actually be in before ever adopting Lineage's conventions. None of
// them reach a structured skills/<name>/SKILL.md + workflows/<name>/WORKFLOW.md
// layout on purpose: that shape is already covered by
// internal/packages.Discover, and isn't what #102 needs to prove out.
//
// Level 1: one file, everything inline (scripts as fenced code blocks).
// Level 2: root instructions + flat scripts/ folder, referenced by bare name.
// Level 3: root instructions + scripts referenced by path + a stray README.
// Level 4: split into multiple instruction-shaped files with non-canonical
//          names (AGENT.md, not AGENTS.md; GATHER_NOTES.md, not SKILL.md).
// Level 5: proto-skill folders (skills/<name>/README.md instead of
//          SKILL.md; workflows/<name>.md instead of workflows/<name>/WORKFLOW.md).

func TestMessinessLevel1_SingleFileHasNoSeparateArtifacts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), `# Resume Helper

This project helps review resumes against a target role.

## Step 1: Gather Notes

Confirm the target role and ask the receiver to fill in a notes template
with the role, measurable achievements, and constraints to avoid.

`+"```python\nimport csv\n\ndef load_notes(path):\n    with open(path) as f:\n        return list(csv.DictReader(f))\n```"+`

## Step 2: Resume Review

Review the supplied resume material for clarity, evidence, role fit, and
missing details.

`+"```python\ndef review(resume_text, notes):\n    return {\"clarity\": 0, \"evidence\": 0}\n```"+`
`)

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// A genuinely single-file workspace has nothing else to discover: the
	// scripts and the second "skill" only exist as prose/code-fence
	// subsections inside one file, invisible to file-level classification.
	// This is the hard floor of what #102 can do here — parsing internal
	// markdown structure to recover the embedded steps is #104's job, not
	// this package's. The test documents that boundary, not a bug.
	if len(inv.Entries) != 1 {
		t.Fatalf("Entries = %d, want exactly 1 (single-file workspace)", len(inv.Entries))
	}
	if inv.Entries[0].Kind != KindInstruction {
		t.Errorf("Kind = %q, want %q", inv.Entries[0].Kind, KindInstruction)
	}
}

func TestMessinessLevel2_FlatScriptsCitedByBareName(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), `# Resume Helper

Step 1: run gather_notes.py to collect the target role and source material
into notes_template.csv.

Step 2: run review_resume.py to review the resume against the collected
notes.
`)
	mustWrite(t, filepath.Join(root, "scripts", "gather_notes.py"), "import csv\n")
	mustWrite(t, filepath.Join(root, "scripts", "review_resume.py"), "def review():\n    pass\n")
	mustWrite(t, filepath.Join(root, "notes_template.csv"), "role,achievement,constraint\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	claude := byPath["CLAUDE.md"]
	if len(claude.Mentions) != 3 {
		t.Fatalf("CLAUDE.md Mentions = %+v, want 3 (both scripts + the csv, all bare-name matches)", claude.Mentions)
	}
	for _, p := range []string{"scripts/gather_notes.py", "scripts/review_resume.py", "notes_template.csv"} {
		if len(byPath[p].ReferencedBy) != 1 {
			t.Errorf("%s ReferencedBy = %+v, want exactly 1", p, byPath[p].ReferencedBy)
		}
	}
}

func TestMessinessLevel3_PathReferencesAndRootReadmeStaysSetup(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "CLAUDE.md"), `# Resume Helper

1. Run scripts/gather_notes.py. It writes to notes/source-notes-template.csv.
2. Run scripts/review_resume.py against that file.
`)
	mustWrite(t, filepath.Join(root, "README.md"), "# Setup\n\npip install -r requirements.txt\n")
	mustWrite(t, filepath.Join(root, "scripts", "gather_notes.py"), "import csv\n")
	mustWrite(t, filepath.Join(root, "scripts", "review_resume.py"), "def review():\n    pass\n")
	mustWrite(t, filepath.Join(root, "notes", "source-notes-template.csv"), "role,achievement,constraint\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	if len(byPath["CLAUDE.md"].Mentions) != 3 {
		t.Errorf("CLAUDE.md Mentions = %+v, want 3", byPath["CLAUDE.md"].Mentions)
	}
	// A root-level README, not located under skills/ or workflows/, is
	// setup material — not reclassified as instruction just because it's
	// markdown. Location, not just extension, decides this.
	if byPath["README.md"].Kind != KindSetupMaterial {
		t.Errorf("README.md Kind = %q, want %q", byPath["README.md"].Kind, KindSetupMaterial)
	}
}

func TestMessinessLevel4_NonCanonicalInstructionFilesStillCite(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "AGENT.md"), `# Resume Review Agent

See GATHER_NOTES.md then RESUME_REVIEW.md.
Scripts: scripts/gather_notes.py, scripts/review_resume.py.
Data: references/source-notes-template.csv.
`)
	mustWrite(t, filepath.Join(root, "GATHER_NOTES.md"), "Confirm the target role and fill in references/source-notes-template.csv.\nCalls scripts/gather_notes.py.\n")
	mustWrite(t, filepath.Join(root, "RESUME_REVIEW.md"), "Review the resume for clarity, evidence, role fit.\nCalls scripts/review_resume.py.\n")
	mustWrite(t, filepath.Join(root, "scripts", "gather_notes.py"), "import csv\n")
	mustWrite(t, filepath.Join(root, "scripts", "review_resume.py"), "def review():\n    pass\n")
	mustWrite(t, filepath.Join(root, "references", "source-notes-template.csv"), "role,achievement,constraint\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	// None of these three match the canonical CLAUDE.md/AGENTS.md/SKILL.md/
	// WORKFLOW.md names, and none live under a skills/workflows directory,
	// so classify() honestly leaves them KindUnknown rather than guessing.
	for _, p := range []string{"AGENT.md", "GATHER_NOTES.md", "RESUME_REVIEW.md"} {
		if byPath[p].Kind != KindUnknown {
			t.Errorf("%s Kind = %q, want %q", p, byPath[p].Kind, KindUnknown)
		}
	}

	// But Kind == unknown must not mean "excluded from citation scanning":
	// this is the core regression. AGENT.md's prose literally names all 5
	// other files; that evidence must not be lost just because AGENT.md
	// itself couldn't be confidently classified.
	if len(byPath["AGENT.md"].Mentions) != 5 {
		t.Fatalf("AGENT.md Mentions = %+v, want 5 (2 sub-files + 2 scripts + 1 reference)", byPath["AGENT.md"].Mentions)
	}
	if len(byPath["GATHER_NOTES.md"].Mentions) != 2 {
		t.Errorf("GATHER_NOTES.md Mentions = %+v, want 2", byPath["GATHER_NOTES.md"].Mentions)
	}
	if len(byPath["GATHER_NOTES.md"].ReferencedBy) != 1 {
		t.Errorf("GATHER_NOTES.md ReferencedBy = %+v, want 1 (cited by AGENT.md)", byPath["GATHER_NOTES.md"].ReferencedBy)
	}
	if len(byPath["scripts/gather_notes.py"].ReferencedBy) != 2 {
		t.Errorf("scripts/gather_notes.py ReferencedBy = %+v, want 2 (AGENT.md + GATHER_NOTES.md)", byPath["scripts/gather_notes.py"].ReferencedBy)
	}
}

func TestMessinessLevel5_ProtoSkillFoldersClassifyByLocation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "skills", "gather-notes", "README.md"),
		"Collect the target role and source material. Run gather_notes.py, writing to ../../references/source-notes-template.csv.\n")
	mustWrite(t, filepath.Join(root, "skills", "gather-notes", "gather_notes.py"), "import csv\n")
	mustWrite(t, filepath.Join(root, "skills", "resume-review", "README.md"),
		"Review the resume for clarity, evidence, role fit. Run review_resume.py.\n")
	mustWrite(t, filepath.Join(root, "skills", "resume-review", "review_resume.py"), "def review():\n    pass\n")
	mustWrite(t, filepath.Join(root, "workflows", "resume-review.md"),
		"1. gather-notes\n2. resume-review\n")
	mustWrite(t, filepath.Join(root, "references", "source-notes-template.csv"), "role,achievement,constraint\n")

	inv, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	byPath := entryMap(inv)

	// A README.md nested inside skills/<name>/ is that skill's instruction
	// content, not a generic project setup note — location beats the bare
	// "README" filename prefix. Same for a workflow file that lives
	// directly under workflows/ without the canonical per-workflow
	// directory + WORKFLOW.md shape.
	for _, p := range []string{"skills/gather-notes/README.md", "skills/resume-review/README.md", "workflows/resume-review.md"} {
		if byPath[p].Kind != KindInstruction {
			t.Errorf("%s Kind = %q, want %q", p, byPath[p].Kind, KindInstruction)
		}
	}

	// The workflow file only names its steps by skill name ("gather-notes",
	// "resume-review"), never by an actual file path or filename — so it
	// correctly produces zero citations. Resolving "gather-notes" as a
	// reference to the skills/gather-notes/ directory requires understanding
	// a naming convention, not just matching a literal string; that's
	// exactly the paraphrase-style gap this package intentionally leaves to
	// #104, documented in the package doc comment.
	if len(byPath["workflows/resume-review.md"].Mentions) != 0 {
		t.Errorf("workflows/resume-review.md Mentions = %+v, want 0 (skill names aren't literal file paths)", byPath["workflows/resume-review.md"].Mentions)
	}

	if len(byPath["skills/gather-notes/README.md"].Mentions) != 2 {
		t.Errorf("skills/gather-notes/README.md Mentions = %+v, want 2 (script + csv)", byPath["skills/gather-notes/README.md"].Mentions)
	}
}
