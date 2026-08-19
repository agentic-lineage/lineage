package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const WorkflowFileName = "WORKFLOW.md"

// Workflow is an ordered sequence of skill names, declared in a
// WORKFLOW.md's YAML frontmatter (the same `---`-delimited convention
// SKILL.md already uses). A workflow with no frontmatter, or frontmatter
// with no `steps`, has an empty Steps list — WORKFLOW.md remains valid
// plain documentation on its own, same as before this existed.
type Workflow struct {
	Name  string
	Steps []string
}

// LoadWorkflow reads and parses the WORKFLOW.md frontmatter for the named
// workflow inside pkgDir.
func LoadWorkflow(pkgDir, name string) (Workflow, error) {
	path := filepath.Join(pkgDir, "workflows", name, WorkflowFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}

	front, err := workflowFrontmatter(data)
	if err != nil {
		return Workflow{}, fmt.Errorf("parse %s: %w", path, err)
	}

	var parsed struct {
		Steps []string `yaml:"steps"`
	}
	if len(front) > 0 {
		if err := yaml.Unmarshal(front, &parsed); err != nil {
			return Workflow{}, fmt.Errorf("parse %s frontmatter: %w", path, err)
		}
	}
	return Workflow{Name: name, Steps: parsed.Steps}, nil
}

// workflowFrontmatter extracts the YAML block between a leading "---" line
// and the next "---" line. Content with no frontmatter (no leading "---")
// returns nil, nil rather than an error, since a plain-prose WORKFLOW.md is
// valid and simply declares no steps.
func workflowFrontmatter(data []byte) ([]byte, error) {
	const delim = "---"
	text := string(data)
	if !strings.HasPrefix(text, delim+"\n") {
		return nil, nil
	}
	rest := text[len(delim)+1:]
	end := strings.Index(rest, "\n"+delim)
	if end == -1 {
		return nil, fmt.Errorf("unterminated frontmatter (missing closing %q)", delim)
	}
	return []byte(rest[:end]), nil
}

// ValidateWorkflowSteps checks that every step in wf references a skill
// pkg actually has. Returns an error naming the first missing step.
func ValidateWorkflowSteps(pkg Package, wf Workflow) error {
	if missing := missingDeclared(wf.Steps, pkg.Skills); len(missing) > 0 {
		return fmt.Errorf("workflow %q references skill %q, which package %q does not have", wf.Name, missing[0], pkg.Manifest.Name)
	}
	return nil
}

// FindWorkflow looks for a workflow named name across pkgs (typically the
// set of currently-enabled packages) and returns the package that declares
// it along with the parsed workflow. It's an error if no enabled package
// declares the workflow, or if more than one does — workflow names aren't
// namespaced across packages, so an ambiguous match needs a human decision
// rather than a silent pick.
func FindWorkflow(pkgs []Package, name string) (Package, Workflow, error) {
	var ownerPkg Package
	var found bool

	for _, pkg := range pkgs {
		for _, wfName := range pkg.Workflows {
			if wfName != name {
				continue
			}
			if found {
				return Package{}, Workflow{}, fmt.Errorf("workflow %q is declared by more than one enabled package (%q and %q); this isn't supported yet", name, ownerPkg.Manifest.Name, pkg.Manifest.Name)
			}
			ownerPkg = pkg
			found = true
		}
	}
	if !found {
		return Package{}, Workflow{}, fmt.Errorf("workflow %q not found in any enabled package", name)
	}

	wf, err := LoadWorkflow(ownerPkg.Path, name)
	if err != nil {
		return Package{}, Workflow{}, fmt.Errorf("load workflow %q: %w", name, err)
	}
	if err := ValidateWorkflowSteps(ownerPkg, wf); err != nil {
		return Package{}, Workflow{}, err
	}
	return ownerPkg, wf, nil
}
