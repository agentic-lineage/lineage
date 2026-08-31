// Package inventory walks an arbitrary, possibly-messy source workspace —
// one that is not yet a valid Lineage package — and produces a deterministic,
// read-only inventory of what's there: every file's kind, why it was
// classified that way, and which markdown files (whether or not classify()
// could confidently name their role — canonical instruction files like
// CLAUDE.md/AGENTS.md/SKILL.md/WORKFLOW.md, or an unclassified README-style
// file that still contains real prose) literally name it in their text.
//
// This package deliberately does not interpret, execute, or understand
// intent. Citation matching (see Citation) is plain string/path matching
// against literal filenames — it catches "run scripts/deploy.sh" but not
// "run the deploy script" with no filename in it. That gap is intentional:
// resolving paraphrased or implicit references requires real understanding,
// which belongs to a later, model-assisted analysis stage that consumes
// this inventory (including Mentions) plus raw file content. Treat Mentions
// and ReferencedBy as a precise but incomplete evidence trail, never as a
// complete call graph.
//
// Symlinks are refused rather than followed, including one passed as the
// discovery root itself: an inventory reports on the tree literally named,
// and quietly resolving elsewhere would make Inventory.Root a different path
// from the one the caller asked about.
//
// Non-goals for a consumer of this evidence, worth stating up front rather
// than discovering by producing wrong conclusions:
//
//   - ToPath is the only field on Citation that names a location. AsWritten
//     is descriptive, not resolvable — for a MatchBasename edge it is a
//     partial reference ("foo/run.sh" for skills/foo/run.sh) that cannot be
//     turned into the real path by joining it against FromPath's directory.
//     Re-deriving a location from AsWritten reproduces, at the consuming
//     layer, exactly the navigation-resolution bugs this package's own
//     matcher was hardened against; use ToPath.
//   - Citation.Column is a byte offset, not a rune or character offset. A
//     consumer that indexes by codepoint (as most languages other than Go
//     do by default) will misalign on any line containing non-ASCII text.
//   - Only markdown (.md) files are scanned for citations. A file whose only
//     real reference is structural — e.g. named in a lineage.yaml manifest's
//     entrypoints field, arguably stronger evidence than prose — produces no
//     Citation at all today. An empty ReferencedBy on such a file is not
//     evidence it is unused, the same caution AmbiguousBasename gives for a
//     different reason; this package simply does not look there yet.
package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type ArtifactKind string

const (
	KindInstruction      ArtifactKind = "instruction"
	KindExecutableHelper ArtifactKind = "executable_helper"
	KindReference        ArtifactKind = "reference"
	KindSetupMaterial    ArtifactKind = "setup_material"
	KindPackageMetadata  ArtifactKind = "package_metadata"
	KindUnknown          ArtifactKind = "unknown"
)

// MatchKind records how a citation was found, which is a statement about how
// strong the evidence is rather than how certain it is. A consumer weighing
// evidence should treat the two differently.
type MatchKind string

const (
	// MatchPath means the prose named the entry's full relative path (possibly
	// reached through "./" or "../"). The most specific claim available.
	MatchPath MatchKind = "path"
	// MatchBasename means the prose named a proper suffix of the entry's path:
	// a bare filename, or a partial path like "foo/run.sh" for
	// "skills/foo/run.sh". Weaker, and only offered when the entry's basename
	// is unique in the tree, since otherwise a suffix names several files.
	//
	// A bare filename with no extension — "run", "build" — is never cited at
	// all, by any match kind, unless named through a "/" or a "."/".." prefix:
	// stripped of any path separator it is indistinguishable from an English
	// word in ordinary prose. This applies even to such a file sitting at the
	// workspace root, where a bare mention is technically also its whole
	// path — root placement doesn't make the word any less ambiguous.
	MatchBasename MatchKind = "basename"
)

// Citation is one directed edge in the evidence graph: markdown file FromPath
// names inventoried artifact ToPath, on this line, by this literal text. It
// is the "why is this in scope" evidence, sized to be cheap to carry into an
// LLM prompt later without re-opening source files.
//
// The same Citation value is recorded twice — on the citer's Mentions and on
// the target's ReferencedBy — so the graph can be walked from either end.
// ToPath is therefore redundant on the ReferencedBy side; that redundancy is
// what makes a single edge self-describing wherever it is read.
type Citation struct {
	FromPath  string    `json:"from_path"`  // the markdown file doing the citing
	ToPath    string    `json:"to_path"`    // the inventoried artifact being named — the only field naming a location
	Line      int       `json:"line"`       // 1-indexed line number within FromPath
	MatchKind MatchKind `json:"match_kind"` // how it matched, i.e. how strong the evidence is

	// Column is a 1-indexed BYTE offset into the raw source line, where
	// AsWritten begins — not a rune or character offset, so a consumer
	// indexing by codepoint will misalign on a non-ASCII line. Note also
	// that Snippet is trimmed and re-windowed, so Column does not index
	// into it either.
	Column int `json:"column"`

	// AsWritten is the reference exactly as the prose wrote it, which is
	// descriptive, not resolvable. It is not always ToPath: a line naming
	// "../../references/data.csv" cites references/data.csv but wrote the
	// relative prefix, and a line naming "foo/run.sh" (MatchBasename) wrote
	// a partial reference to skills/foo/run.sh, not a location that can be
	// joined against FromPath's directory to get there. Use ToPath for the
	// location; use AsWritten only to show or reason about how the citing
	// prose phrased the reference.
	AsWritten string `json:"as_written"`

	Snippet string `json:"snippet"` // preview of the source line; always contains AsWritten
}

// maxSnippetLen bounds Citation.Snippet so a citation is always small and
// boundable, regardless of how long the source line actually is.
const maxSnippetLen = 200

// Entry describes one file discovered under a workspace root.
type Entry struct {
	Path string       `json:"path"` // relative to Inventory.Root, forward-slashed
	Kind ArtifactKind `json:"kind"`
	// Reason is the base classification reason (the heuristic rule that
	// matched). It is independent of citations and is never overwritten by
	// the cross-reference pass.
	Reason   string `json:"reason"`
	Digest   string `json:"digest"` // "sha256:<hex>" over this file's content
	Size     int64  `json:"size"`
	Language string `json:"language,omitempty"` // by extension, best-effort

	// AmbiguousBasename is true when this file's basename is shared with at
	// least one other file in the tree. A partial reference — a bare filename,
	// or any suffix short of the full path — would name several files, so it is
	// suppressed for this entry (see buildCandidates) and only an exact path
	// mention can cite it.
	//
	// Without this flag an empty ReferencedBy is ambiguous: a consumer cannot
	// tell a genuinely unreferenced file from one whose weak matches were
	// deliberately withheld, and must not conclude "unused" from the second
	// case. Reported honestly rather than papered over.
	AmbiguousBasename bool `json:"ambiguous_basename,omitempty"`

	// Mentions is populated for every markdown (.md) entry, regardless of
	// Kind: the outgoing edges from this file, one Citation per (line,
	// target) pair naming another inventoried artifact. Read ToPath to learn
	// what was named. Citation eligibility depends only on being prose, not
	// on whether classify() could confidently label the citing file itself —
	// an unclassified markdown file can still cite.
	Mentions []Citation `json:"mentions,omitempty"`

	// ReferencedBy is populated on the target side, any kind: the incoming
	// edges naming this entry. Empty means literally unreferenced by any
	// markdown file's prose — a real, honestly-reported fact, not an
	// omission — but read AmbiguousBasename before drawing a conclusion
	// from it.
	ReferencedBy []Citation `json:"referenced_by,omitempty"`
}

// CurrentSchema is the version of the serialized Inventory shape, named and
// tagged to match packages.Manifest.Schema and snapshot manifests per ADR
// 0005. Discover stamps it on every result so a later consumer reading a
// stored inventory can tell which field set it was written with.
//
// Unlike those two, nothing reads a stored inventory back yet, so there is no
// parse-time schema check here and no "0 means 1" defaulting. Whoever adds
// the first reader should add both, following packages.LoadManifest.
const CurrentSchema = 1

// Inventory is the deterministic result of Discover.
type Inventory struct {
	Schema  int     `json:"schema"`
	Root    string  `json:"root"`
	Entries []Entry `json:"entries"` // sorted by Path
}

// defaultIgnoredDirs are directory names skipped anywhere in the tree by
// default: version control, dependency folders, and common build output.
var defaultIgnoredDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"vendor":        true,
	"dist":          true,
	"build":         true,
	"venv":          true,
	".venv":         true,
	"__pycache__":   true,
	".pytest_cache": true,
	".mypy_cache":   true,
	".tox":          true,
	".idea":         true,
	".vscode":       true,
	"target":        true,
	".next":         true,
}

// Discover walks root read-only and returns a deterministic inventory. It
// never creates, edits, deletes, or executes anything under root.
func Discover(root string) (Inventory, error) {
	inv := Inventory{Schema: CurrentSchema, Root: root}

	// The discovery root is an argument to validate, not a tree entry: the
	// walk below deliberately skips it (a workspace directory that happens to
	// be named "vendor" must not match defaultIgnoredDirs and scan nothing),
	// so preconditions on it belong here rather than in the callback. Without
	// this, a symlinked or non-directory root walked nothing and returned a
	// successful empty inventory — indistinguishable from a clean scan of an
	// empty workspace.
	//
	// Symlinks are refused rather than resolved so Inventory.Root stays the
	// path the caller actually named.
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return Inventory{}, fmt.Errorf("stat discovery root %s: %w", root, err)
	}
	if rootInfo.Mode()&fs.ModeSymlink != 0 {
		return Inventory{}, fmt.Errorf("refusing to use symlink %s as the discovery root", root)
	}
	if !rootInfo.IsDir() {
		return Inventory{}, fmt.Errorf("discovery root %s is not a directory", root)
	}

	var relPaths []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if isIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing to include symlink %s", rel)
		}
		if !d.Type().IsRegular() {
			return nil
		}

		relPaths = append(relPaths, rel)
		return nil
	})
	if err != nil {
		return Inventory{}, err
	}
	sort.Strings(relPaths)

	entries := make(map[string]*Entry, len(relPaths))
	for _, rel := range relPaths {
		full := filepath.Join(root, filepath.FromSlash(rel))
		digest, size, digestErr := digestFile(full)
		if digestErr != nil {
			return Inventory{}, fmt.Errorf("digest %s: %w", rel, digestErr)
		}

		kind, reason := classify(rel)
		entries[rel] = &Entry{
			Path:     rel,
			Kind:     kind,
			Reason:   reason,
			Digest:   digest,
			Size:     size,
			Language: languageFor(rel),
		}
	}

	if err := crossReference(root, relPaths, entries); err != nil {
		return Inventory{}, err
	}

	result := make([]Entry, 0, len(entries))
	for _, rel := range relPaths {
		result = append(result, *entries[rel])
	}
	inv.Entries = result
	return inv, nil
}

func isIgnoredDir(name string) bool {
	return defaultIgnoredDirs[name]
}

// digestFile hashes a file's content and reports its size. The size comes from
// the bytes just read rather than a separate stat: the walk has already
// established this is a regular file, so the two always agree, and this drops
// a syscall and an error branch per file.
func digestFile(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:]), int64(len(data)), nil
}
