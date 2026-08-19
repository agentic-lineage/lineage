package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lineage-dev/lineage/internal/config"
)

type Plan struct {
	Name   string
	Binary string
	Args   []string
	Env    []string
}

func Resolve(name, lineageHome string, project config.ProjectConfig, args []string) (Plan, error) {
	cfg := project.Providers[name]
	binary := cfg.Binary
	if binary == "" {
		var err error
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
	pathEnv := os.Getenv("PATH")
	shims, _ := filepath.Abs(config.ShimsDir(lineageHome))

	var candidates []string
	for _, dir := range filepath.SplitList(pathEnv) {
		absDir, err := filepath.Abs(dir)
		if err == nil && absDir == shims {
			continue
		}

		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates
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
