package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lineage-dev/lineage/internal/config"
	"github.com/lineage-dev/lineage/internal/packages"
	"github.com/lineage-dev/lineage/internal/provider"
	"github.com/lineage-dev/lineage/internal/runtime"
	"github.com/lineage-dev/lineage/internal/shim"
)

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	home, err := config.HomeDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], home, stdout, stderr)
	case "package":
		return runPackage(args[1:], stdout, stderr)
	case "enable":
		return runEnable(args[1:], home, stdout, stderr)
	case "run":
		return runProvider(ctx, args[1:], home, stdout, stderr)
	case "install-shims":
		return runInstallShims(home, stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		err := fmt.Errorf("unknown command %q", args[0])
		fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return err
	}
}

func runInit(args []string, home string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		err := fmt.Errorf("usage: lineage init user | lineage init workspace <name>")
		fmt.Fprintln(stderr, err)
		return err
	}

	switch args[0] {
	case "user":
		if err := os.MkdirAll(config.UserPackagesDir(home), 0o755); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		fmt.Fprintf(stdout, "initialized user packages at %s\n", config.UserPackagesDir(home))
		return nil
	case "workspace":
		if len(args) != 2 {
			err := fmt.Errorf("usage: lineage init workspace <name>")
			fmt.Fprintln(stderr, err)
			return err
		}
		dir := config.WorkspacePackagesDir(home, args[1])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		fmt.Fprintf(stdout, "initialized workspace %s at %s\n", args[1], dir)
		return nil
	default:
		err := fmt.Errorf("unknown init target %q", args[0])
		fmt.Fprintln(stderr, err)
		return err
	}
}

func runPackage(args []string, stdout, stderr io.Writer) error {
	if len(args) != 2 {
		err := fmt.Errorf("usage: lineage package init <name> | lineage package validate <path>")
		fmt.Fprintln(stderr, err)
		return err
	}

	switch args[0] {
	case "init":
		name := args[1]
		dir := filepath.Clean(name)
		if err := packages.InitPackage(dir, filepath.Base(name)); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		fmt.Fprintf(stdout, "initialized package %s\n", dir)
		return nil
	case "validate":
		return runPackageValidate(filepath.Clean(args[1]), stdout, stderr)
	default:
		err := fmt.Errorf("unknown package command %q", args[0])
		fmt.Fprintln(stderr, err)
		return err
	}
}

func runPackageValidate(dir string, stdout, stderr io.Writer) error {
	report, err := packages.Validate(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "package: %s@%s (schema %d)\n", report.Manifest.Name, report.Manifest.Version, report.Manifest.Schema)
	fmt.Fprintf(stdout, "digest: %s\n", emptyValue(report.Digest))
	fmt.Fprintf(stdout, "capabilities:\n")
	fmt.Fprintf(stdout, "  filesystem.read: %s\n", listValue(report.Manifest.Capabilities.Filesystem.Read))
	fmt.Fprintf(stdout, "  network: %s\n", listValue(report.Manifest.Capabilities.Network))

	if len(report.Notes) > 0 {
		fmt.Fprintf(stdout, "notes:\n")
		for _, note := range report.Notes {
			fmt.Fprintf(stdout, "  - %s\n", note)
		}
	}

	if !report.Passed() {
		fmt.Fprintf(stdout, "errors:\n")
		for _, e := range report.Errors {
			fmt.Fprintf(stdout, "  - %s\n", e)
		}
		fmt.Fprintf(stdout, "result: FAIL\n")
		err := fmt.Errorf("package validation failed with %d error(s)", len(report.Errors))
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "result: PASS\n")
	return nil
}

func listValue(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func emptyValue(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func runEnable(args []string, home string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		err := fmt.Errorf("usage: lineage enable <package-path-or-id>")
		fmt.Fprintln(stderr, err)
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Reuse the project config an enclosing `lineage run` would find (it
	// walks up parent directories), rather than always looking at cwd. This
	// keeps `enable` and `run` agreeing on where "the project" is: enabling
	// a package from inside a subdirectory of an already-initialized
	// project must update that project's existing root config, not fork a
	// second, shadow .lineage/config.yaml next to cwd.
	projectRoot := cwd
	configPath := config.ProjectConfigPath(cwd)
	cfg := config.DefaultProjectConfig()
	if found, err := config.FindProjectConfig(cwd); err == nil {
		projectRoot = found.Root
		configPath = found.Path
		cfg = found.Config
	} else if !errors.Is(err, config.ErrProjectConfigNotFound) {
		fmt.Fprintln(stderr, err)
		return err
	}

	ref := args[0]

	// A "./" or "../" style ref is relative to where the user typed it
	// (cwd), matching the documented `lineage enable ./resume-workflow`
	// usage. A bare name is a lookup by id against the project/user/
	// workspace package search path, which is anchored at the project
	// root. Resolve each the way it's meant, so a relative ref keeps
	// pointing at the exact directory the user meant even when cwd is a
	// subdirectory of a project that was already initialized higher up.
	resolveRoot := projectRoot
	if filepath.IsAbs(ref) || strings.HasPrefix(ref, ".") {
		resolveRoot = cwd
	}
	resolvedPath, err := packages.ResolveReference(home, cfg.Workspace, resolveRoot, ref)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// The config we're about to save lives at the project root, and
	// `lineage run` later resolves every enabled_packages entry relative to
	// that same root. Store relative refs re-expressed against projectRoot
	// so they still resolve correctly, instead of the literal string the
	// user typed relative to cwd.
	storedRef := ref
	if strings.HasPrefix(ref, ".") && resolveRoot != projectRoot {
		if rel, relErr := filepath.Rel(projectRoot, resolvedPath); relErr == nil {
			rel = filepath.ToSlash(rel)
			if !strings.HasPrefix(rel, ".") {
				rel = "./" + rel
			}
			storedRef = rel
		}
	}

	newEnabled := cfg.EnabledPackages
	if !contains(newEnabled, storedRef) {
		newEnabled = append(append([]string{}, newEnabled...), storedRef)
	}

	// Validate against the full set that would be enabled after this
	// change, not just the package being added: a required skill can
	// legitimately come from a different already-enabled package.
	resolvedForValidation, err := packages.ResolveEnabled(home, cfg.Workspace, projectRoot, newEnabled)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if err := packages.ValidateDependencies(resolvedForValidation); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	cfg.EnabledPackages = newEnabled
	if err := config.SaveProjectConfig(configPath, cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	fmt.Fprintf(stdout, "enabled package %s in %s\n", storedRef, configPath)
	return nil
}

func runProvider(ctx context.Context, args []string, home string, stdout, stderr io.Writer) error {
	_ = ctx
	if len(args) == 0 {
		err := fmt.Errorf("usage: lineage run <%s> [--dry-run] [-- provider args...]", providerNameList())
		fmt.Fprintln(stderr, err)
		return err
	}

	providerName := args[0]
	dryRun, providerArgs := parseRunArgs(args[1:])

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	plan, err := runtime.BuildPlan(providerName, cwd, home, providerArgs)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if dryRun {
		fmt.Fprint(stdout, plan.DryRunString())
		return nil
	}
	if err := provider.Launch(plan.ProviderPlan); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	return nil
}

func runInstallShims(home string, stdout, stderr io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if err := shim.Install(home, exe); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	fmt.Fprintf(stdout, "installed shims in %s\n", config.ShimsDir(home))
	fmt.Fprintf(stdout, "add this directory before existing agent binaries in PATH to enable transparent Lineage runtime\n")
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(fmt.Sprintf(`
Lineage shareable agent runtime

Usage:
  lineage init user
  lineage init workspace <name>
  lineage package init <name>
  lineage package validate <path>
  lineage enable <package-path-or-id>
  lineage run <%s> [--dry-run] [-- provider args...]
  lineage install-shims
`, providerNameList())))
}

// providerNameList returns every registered provider name, comma-separated,
// for CLI usage messages — so adding a provider to internal/provider's
// registry is enough to keep help text accurate, with no other file to edit.
func providerNameList() string {
	known := provider.Known()
	names := make([]string, len(known))
	for i, p := range known {
		names[i] = p.Name
	}
	return strings.Join(names, "|")
}

// parseRunArgs splits the arguments following `lineage run <provider>` into
// lineage's own --dry-run flag and the args that should be forwarded to the
// wrapped provider. A bare "--" marks the boundary explicitly: everything
// before it is scanned for lineage flags, everything after it is passed
// through to the provider verbatim, even if it happens to collide with a
// lineage flag name (for example a provider's own --dry-run flag). Without
// "--", the whole argument list is scanned for lineage flags, matching the
// simple common case of `lineage run claude --dry-run`.
func parseRunArgs(args []string) (dryRun bool, providerArgs []string) {
	providerArgs = []string{}

	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}

	scan := args
	var passthrough []string
	if sep >= 0 {
		scan = args[:sep]
		passthrough = args[sep+1:]
	}

	for _, arg := range scan {
		if arg == "--dry-run" {
			dryRun = true
			continue
		}
		providerArgs = append(providerArgs, arg)
	}
	providerArgs = append(providerArgs, passthrough...)
	return dryRun, providerArgs
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
