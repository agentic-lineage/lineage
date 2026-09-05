package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentic-lineage/lineage/internal/config"
)

type Plan struct {
	Name            string
	Binary          string
	Args            []string
	Env             []string
	MaterializeOnly bool
}

func Resolve(name, lineageHome string, project config.ProjectConfig, args []string) (Plan, error) {
	adapter, err := Get(name)
	if err != nil {
		return Plan{}, err
	}
	if adapter.MaterializeOnly {
		return Plan{Name: name, Args: append([]string{}, args...), MaterializeOnly: true}, nil
	}

	cfg := project.Providers[name]
	binary := cfg.Binary
	if binary == "" {
		binary, err = findRealBinary(name, lineageHome)
		if err != nil {
			return Plan{}, err
		}
	}

	mergedArgs := append([]string{}, cfg.Args...)
	mergedArgs = append(mergedArgs, args...)
	return Plan{
		Name:   name,
		Binary: binary,
		Args:   mergedArgs,
		Env: append(os.Environ(),
			"LINEAGE_ACTIVE=1",
			"LINEAGE_PROVIDER="+name,
		),
	}, nil
}

func Launch(plan Plan) error {
	cmd := exec.Command(plan.Binary, plan.Args...)
	cmd.Env = plan.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findRealBinary(name, lineageHome string) (string, error) {
	candidates := CandidateBinaries(name, lineageHome)
	if len(candidates) == 0 {
		return "", fmt.Errorf("real %s binary not found; set providers.%s.binary in .lineage/config.yaml", name, name)
	}
	return candidates[0], nil
}

// CandidateBinaries returns every executable named name found on PATH,
// in PATH search order, skipping the lineage shim directory itself (the
// same rule findRealBinary uses to pick the one it actually launches).
// Used by `lineage doctor` to warn when more than one real binary exists
// for a provider — findRealBinary picking the first one silently is
// exactly the kind of ambiguity that's invisible until it picks wrong.
func CandidateBinaries(name, lineageHome string) []string {
	return candidateBinariesFor(name, lineageHome, runtime.GOOS)
}

// candidateBinariesFor is CandidateBinaries with an explicit goos, so the
// Windows-specific matching (PATHEXT-based extensions, no executable-bit
// check) is testable without actually running on Windows.
func candidateBinariesFor(name, lineageHome, goos string) []string {
	pathEnv := os.Getenv("PATH")
	shims, _ := filepath.Abs(config.ShimsDir(lineageHome))
	isWindows := goos == "windows"

	var candidates []string
	for _, dir := range filepath.SplitList(pathEnv) {
		absDir, err := filepath.Abs(dir)
		if err == nil && absDir == shims {
			continue
		}

		for _, ext := range candidateExtensions(goos) {
			candidate := filepath.Join(dir, name+ext)
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			// Windows has no POSIX executable bit - PATHEXT membership is
			// what makes a file "runnable" as a bare command there.
			if !isWindows && info.Mode()&0o111 == 0 {
				continue
			}
			// Directory-based exclusion above only catches a shim installed
			// under this call's own lineageHome. A shim installed under a
			// different LINEAGE_HOME earlier, or a stray copy anywhere else
			// on PATH, would otherwise pass through as a "real" binary here
			// - see looksLikeShim's doc comment for why that matters.
			if looksLikeShim(candidate, name) {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

// candidateExtensions returns the filename suffixes to try when resolving
// a bare command name on goos, in priority order. POSIX has none (an exact
// name match, gated on the executable bit); Windows resolves bare commands
// through PATHEXT, so a real provider binary might be name.exe, name.cmd
// (common for npm-installed CLIs), or another PATHEXT entry.
func candidateExtensions(goos string) []string {
	if goos != "windows" {
		return []string{""}
	}
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	return strings.Split(pathext, ";")
}

// shimHeaderPrefixes are the literal opening lines of every shim template
// internal/shim generates. Duplicated here rather than imported, since
// internal/shim already imports this package (provider.Known()) and this
// package importing shim back would be a cycle.
var shimHeaderPrefixes = []string{"#!/bin/sh", "@echo off"}

// looksLikeShim reports whether the file at path is a lineage-generated
// shim script for a provider named providerName, by content rather than
// only by which directory it's in. Directory-based exclusion in
// candidateBinariesFor only catches a shim installed under the current
// call's own lineageHome; a shim installed under a *different*
// LINEAGE_HOME (env var changed between when shims were installed and
// when a later `lineage run` resolves the real binary), or a stray copy
// of the shim script anywhere else on PATH, would otherwise pass through
// as a legitimate "real" binary. Launching that would exec straight back
// into `lineage run`, which resolves the same wrong candidate again: an
// unbounded fork loop, with no self-detection anywhere else in this path.
//
// Requires both a matching header (the exact first line every shim
// template starts with) and the literal "run <providerName>" marker the
// shim body always contains, so an unrelated real script that merely
// starts with "#!/bin/sh" - an extremely common shebang - doesn't false-
// positive; it would also have to coincidentally contain that specific
// marker text.
func looksLikeShim(path, providerName string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	head := string(buf[:n])

	if !strings.Contains(head, "run "+providerName) {
		return false
	}
	for _, prefix := range shimHeaderPrefixes {
		if strings.HasPrefix(head, prefix) {
			return true
		}
	}
	return false
}

func IsShimPath(path, lineageHome string) bool {
	shims, err := filepath.Abs(config.ShimsDir(lineageHome))
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(abs, shims+string(os.PathSeparator))
}
