package packages

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RiskCategory names one class of risky agent instruction this scanner
// looks for. The six categories match the example patterns documented as
// worth flagging for package instruction content — not an invented
// taxonomy.
type RiskCategory string

const (
	CategoryPromptOverride       RiskCategory = "prompt_override"
	CategoryBroadLocalRead       RiskCategory = "broad_local_read"
	CategoryExfiltration         RiskCategory = "exfiltration"
	CategorySilentDestructive    RiskCategory = "silent_destructive"
	CategoryCredentialCollection RiskCategory = "credential_collection"
	CategoryDisableSafety        RiskCategory = "disable_safety"
	// CategoryUnscanned is not a risk finding on its own — it flags a file
	// this scanner could not check at all (too large, unreadable, a
	// symlink target), so "no finding" and "not actually scanned" are
	// never conflated the way a silent skip would.
	CategoryUnscanned RiskCategory = "unscanned"
)

// Severity is how an InstructionFinding should be treated. Block refuses
// export/publish/enable outright, the same posture ScanForSecrets findings
// already get via ValidateReport.Errors. Warn surfaces the finding and
// requires explicit confirmation but does not block on its own.
type Severity string

const (
	SeverityBlock Severity = "block"
	SeverityWarn  Severity = "warn"
)

// InstructionFinding is one risky-instruction match: which file, what kind
// of risk, how severe, and a short human-readable explanation. Excerpt is
// a bounded, single-line quote of the matching line only — never the full
// file — so a finding is always safe to print without risking a second
// disclosure of a secret that happens to sit next to the risky instruction
// in the same file.
type InstructionFinding struct {
	Path     string
	Category RiskCategory
	Severity Severity
	Reason   string
	Excerpt  string
}

// BlockingFindings returns only the findings severe enough to refuse
// export/publish/enable on their own.
func BlockingFindings(findings []InstructionFinding) []InstructionFinding {
	var out []InstructionFinding
	for _, f := range findings {
		if f.Severity == SeverityBlock {
			out = append(out, f)
		}
	}
	return out
}

// WarningFindings returns findings that should be surfaced and require
// explicit confirmation, but do not block on their own. CategoryUnscanned
// findings are deliberately excluded even though they also carry
// SeverityWarn: they aren't a pattern match, and lumping "this looks risky"
// together with "this file couldn't be checked" under one label misstates
// what was actually found. Use UnscannedFindings for those.
func WarningFindings(findings []InstructionFinding) []InstructionFinding {
	var out []InstructionFinding
	for _, f := range findings {
		if f.Severity == SeverityWarn && f.Category != CategoryUnscanned {
			out = append(out, f)
		}
	}
	return out
}

// UnscannedFindings returns findings where a file could not actually be
// checked for instruction risk (too large, unreadable) - not a risk match
// on its own, but still a blind spot a receiver deciding whether to enable
// a package should see and confirm before proceeding.
func UnscannedFindings(findings []InstructionFinding) []InstructionFinding {
	var out []InstructionFinding
	for _, f := range findings {
		if f.Category == CategoryUnscanned {
			out = append(out, f)
		}
	}
	return out
}

// maxScannedInstructionFileSize mirrors maxScannedFileSize in secrets.go:
// content this large isn't the shape of a hand-authored skill/workflow
// file, and reading it into memory for pattern matching has no benefit.
const maxScannedInstructionFileSize = 5 << 20 // 5MB

// maxExcerptLength bounds how much of a matching line ever gets copied
// into a finding's Excerpt (measured in runes, not bytes, so multi-byte
// content is never truncated mid-character).
const maxExcerptLength = 160

// instructionSurfaceDirs are the package subdirectories this scanner
// treats as agent-readable instruction content: the surfaces a package
// can use to influence a receiver's agent through materialized skills,
// workflow steps, agent definitions, policies, and provider adapters.
// references/ is deliberately excluded — it's declared payload data, not
// directives (contrast contentDirNames in discovery.go, which is scoped
// more broadly for digest/export purposes).
var instructionSurfaceDirs = []string{"skills", "workflows", "agents", "policies", "adapters"}

type instructionPattern struct {
	category RiskCategory
	pattern  *regexp.Regexp
	reason   string
}

// instructionPatterns is the small, explicit, documented pattern list this
// scanner runs — the same "precise over exhaustive" tradeoff
// ScanForSecrets already made for credentials (docs/decisions/0009). This
// is a practical risk signal, not a complete prompt-injection defense:
// sophisticated obfuscation will not be caught, and a clean scan is not a
// safety guarantee.
var instructionPatterns = []instructionPattern{
	// Prompt override / jailbreak language.
	{CategoryPromptOverride, regexp.MustCompile(`(?i)ignore\s+(all\s+)?(the\s+)?(previous|prior|above|earlier)\s+instructions`), "content asks the agent to ignore its previous instructions"},
	{CategoryPromptOverride, regexp.MustCompile(`(?i)disregard\s+(the\s+)?(previous|prior|above|earlier)\s+(instructions|prompt|rules)`), "content asks the agent to disregard prior instructions"},
	{CategoryPromptOverride, regexp.MustCompile(`(?i)forget\s+(everything|all)(\s+you\s+were\s+told)?`), "content asks the agent to forget its prior context"},

	// Broad local reads.
	{CategoryBroadLocalRead, regexp.MustCompile(`(?i)read\s+(all|every)\s+files?`), "content asks the agent to read all files, an unusually broad local read"},
	{CategoryBroadLocalRead, regexp.MustCompile(`(?i)scan\s+(the\s+)?(entire\s+)?(home|user)\s+directory`), "content asks the agent to scan the home/user directory"},
	{CategoryBroadLocalRead, regexp.MustCompile(`(?i)list\s+(all\s+)?files\s+(in|under)\s+(the\s+)?(home|root)\s+directory`), "content asks the agent to enumerate the home/root directory"},

	// Network / credential exfiltration.
	{CategoryExfiltration, regexp.MustCompile(`(?i)\b(curl|wget|send|upload|post|exfiltrate|transmit)\b[^\n]{0,60}\b(secret|secrets|token|tokens|password|passwords|credential|credentials|api[- ]?key|\.env)\b`), "content asks the agent to send credential-shaped data over the network"},
	{CategoryExfiltration, regexp.MustCompile(`(?i)\b(secret|secrets|token|tokens|password|passwords|credential|credentials|api[- ]?key)\b[^\n]{0,60}\b(curl|wget|send|upload|post|exfiltrate|transmit)\b`), "content asks the agent to send credential-shaped data over the network"},

	// Silent destructive operations.
	{CategorySilentDestructive, regexp.MustCompile(`(?i)\b(delete|remove|overwrite|wipe)\b[^\n]{0,40}\bwithout\s+(confirmation|asking|prompting|permission)\b`), "content asks the agent to perform a destructive operation without confirmation"},
	{CategorySilentDestructive, regexp.MustCompile(`(?i)silently\s+(delete|remove|overwrite|wipe)`), "content asks the agent to silently perform a destructive operation"},

	// Credential / session-data collection.
	{CategoryCredentialCollection, regexp.MustCompile(`(?i)\b(collect|gather|harvest|extract)\b[^\n]{0,40}\b(passwords?|private\s+keys?|api\s+keys?|session\s+(cookies?|tokens?|data))\b`), "content asks the agent to collect passwords, private keys, or session data"},
	{CategoryCredentialCollection, regexp.MustCompile(`(?i)browser\s+session\s+(data|cookies?|tokens?)`), "content references collecting browser session data"},

	// Disable safety / don't ask.
	{CategoryDisableSafety, regexp.MustCompile(`(?i)disable\s+(all\s+)?safety\s+checks?`), "content asks the agent to disable safety checks"},
	{CategoryDisableSafety, regexp.MustCompile(`(?i)(do\s+not|don['’]?t)\s+ask\s+(the\s+)?user`), "content asks the agent not to ask the user for confirmation"},
	{CategoryDisableSafety, regexp.MustCompile(`(?i)skip\s+(the\s+)?confirmation`), "content asks the agent to skip confirmation"},
	{CategoryDisableSafety, regexp.MustCompile(`(?i)bypass\s+(the\s+)?(safety|permission)\s+checks?`), "content asks the agent to bypass safety/permission checks"},
}

// baseSeverity is a category's severity before the disable-safety pairing
// rule (applyDisableSafetyEscalation) gets a chance to run. This is the
// accepted initial policy: credential/session collection, network or
// credential exfiltration, and silent destructive operations are hard
// stops; prompt-override language and broad local reads are warnings;
// disable-safety language is a warning by default and is only ever
// escalated, never downgraded.
func baseSeverity(category RiskCategory) Severity {
	switch category {
	case CategoryExfiltration, CategorySilentDestructive, CategoryCredentialCollection:
		return SeverityBlock
	default:
		return SeverityWarn
	}
}

// ScanForInstructionRisk walks a package's instruction-bearing surfaces —
// skills, workflows, agents, policies, adapters, and setup.files[].template
// — and flags content matching instructionPatterns. It is a practical risk
// signal, not a complete prompt-injection defense.
//
// setup is passed explicitly because setup.files[].template lives as a
// string inside the manifest, not as a file under any directory this
// scanner otherwise walks — it would be silently missed without this
// separate pass.
func ScanForInstructionRisk(dir string, setup Setup) ([]InstructionFinding, error) {
	var findings []InstructionFinding

	for _, sub := range instructionSurfaceDirs {
		found, err := scanInstructionDir(dir, filepath.Join(dir, sub))
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}

	for _, f := range setup.Files {
		path := fmt.Sprintf("setup.files[%s].template", f.Path)
		findings = append(findings, scanInstructionText(path, f.Template)...)
	}

	findings = applyDisableSafetyEscalation(findings)

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Category < findings[j].Category
	})
	return findings, nil
}

// scanInstructionDir walks root (one of the instruction-surface
// directories under a package) and returns every finding across its
// files. A missing directory — a package that simply doesn't populate a
// policies/ or adapters/ folder, say — is not an error; most packages
// won't use every surface. A symlink is refused outright rather than
// silently skipped or followed, matching how discovery.go and
// materialize.go already treat package-content symlinks. A per-file read
// or size problem becomes a CategoryUnscanned finding rather than being
// dropped with no trace.
func scanInstructionDir(pkgRoot, root string) ([]InstructionFinding, error) {
	var findings []InstructionFinding

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(pkgRoot, path)
		if relErr != nil {
			rel = path
		}
		relSlash := filepath.ToSlash(rel)

		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing to scan symlink %s for instruction risk", relSlash)
		}

		info, statErr := d.Info()
		if statErr != nil {
			findings = append(findings, InstructionFinding{
				Path:     relSlash,
				Category: CategoryUnscanned,
				Severity: SeverityWarn,
				Reason:   fmt.Sprintf("could not stat file for instruction-risk scanning: %v", statErr),
			})
			return nil
		}
		if info.Size() > maxScannedInstructionFileSize {
			findings = append(findings, InstructionFinding{
				Path:     relSlash,
				Category: CategoryUnscanned,
				Severity: SeverityWarn,
				Reason:   "file exceeds the size scanned for instruction risk; its content was not checked",
			})
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			findings = append(findings, InstructionFinding{
				Path:     relSlash,
				Category: CategoryUnscanned,
				Severity: SeverityWarn,
				Reason:   fmt.Sprintf("could not read file for instruction-risk scanning: %v", readErr),
			})
			return nil
		}

		findings = append(findings, scanInstructionText(relSlash, string(data))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}

// scanInstructionText matches instructionPatterns against content (already
// decoded, already read into memory) and returns at most one finding per
// category — the first pattern that matches — so a file that trips
// several patterns in the same category produces one finding, not a wall
// of near-duplicates.
func scanInstructionText(path, content string) []InstructionFinding {
	seen := map[RiskCategory]bool{}
	var findings []InstructionFinding
	for _, p := range instructionPatterns {
		if seen[p.category] {
			continue
		}
		loc := p.pattern.FindStringIndex(content)
		if loc == nil {
			continue
		}
		seen[p.category] = true
		findings = append(findings, InstructionFinding{
			Path:     path,
			Category: p.category,
			Severity: baseSeverity(p.category),
			Reason:   p.reason,
			Excerpt:  excerptAround(content, loc[0], loc[1]),
		})
	}
	return findings
}

// applyDisableSafetyEscalation implements the accepted policy that
// disable-safety language ("disable safety checks", "do not ask the
// user") is a hard stop only when the same file also asks for a
// destructive operation, credential collection, or exfiltration — not on
// its own, where it's common in benign contexts (a debugging skill that
// legitimately says "don't ask me to confirm every read").
func applyDisableSafetyEscalation(findings []InstructionFinding) []InstructionFinding {
	pairedWith := map[string]bool{} // path -> has a destructive/credential/exfil finding
	for _, f := range findings {
		switch f.Category {
		case CategorySilentDestructive, CategoryCredentialCollection, CategoryExfiltration:
			pairedWith[f.Path] = true
		}
	}

	for i := range findings {
		if findings[i].Category != CategoryDisableSafety || !pairedWith[findings[i].Path] {
			continue
		}
		findings[i].Severity = SeverityBlock
		findings[i].Reason += "; paired in the same file with a destructive or credential-related instruction, escalating to a hard stop"
	}
	return findings
}

// excerptAround returns a bounded, single-line quote of content around
// [start,end) — the matched pattern's location — so a finding always
// carries enough context to review without ever including a full file.
// Runs of whitespace collapse to a single space so a match spanning a
// line break still reads as one short quote.
func excerptAround(content string, start, end int) string {
	lineStart := strings.LastIndexByte(content[:start], '\n') + 1
	lineEnd := len(content)
	if rel := strings.IndexByte(content[end:], '\n'); rel >= 0 {
		lineEnd = end + rel
	}

	line := strings.Join(strings.Fields(content[lineStart:lineEnd]), " ")
	if runes := []rune(line); len(runes) > maxExcerptLength {
		line = string(runes[:maxExcerptLength]) + "…"
	}
	return line
}
