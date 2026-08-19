package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lineage-dev/lineage/internal/config"
	"github.com/lineage-dev/lineage/internal/materialize"
	"github.com/lineage-dev/lineage/internal/packages"
	"github.com/lineage-dev/lineage/internal/provider"
	"github.com/lineage-dev/lineage/internal/runtime"
	"github.com/lineage-dev/lineage/internal/shim"
)

func Execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
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
		return runPackage(args[1:], home, stdout, stderr)
	case "enable":
		return runEnable(args[1:], home, stdout, stderr)
	case "list":
		return runList(home, stdout, stderr)
	case "disable":
		return runDisable(args[1:], home, stdout, stderr)
	case "inspect":
		return runInspect(args[1:], home, stdout, stderr)
	case "run":
		return runProvider(ctx, args[1:], home, stdin, stdout, stderr)
	case "install-shims":
		return runInstallShims(home, stdout, stderr)
	case "doctor":
		return runDoctor(home, stdout, stderr)
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

func runPackage(args []string, home string, stdout, stderr io.Writer) error {
	if len(args) < 2 {
		err := fmt.Errorf("usage: lineage package init <name> | lineage package validate <path> | lineage package export <path> [-o file.tgz] | lineage package import <file.tgz> [--as name]")
		fmt.Fprintln(stderr, err)
		return err
	}

	switch args[0] {
	case "init":
		if len(args) != 2 {
			err := fmt.Errorf("usage: lineage package init <name>")
			fmt.Fprintln(stderr, err)
			return err
		}
		name := args[1]
		dir := filepath.Clean(name)
		if err := packages.InitPackage(dir, filepath.Base(name)); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		fmt.Fprintf(stdout, "initialized package %s\n", dir)
		return nil
	case "validate":
		if len(args) != 2 {
			err := fmt.Errorf("usage: lineage package validate <path>")
			fmt.Fprintln(stderr, err)
			return err
		}
		return runPackageValidate(filepath.Clean(args[1]), stdout, stderr)
	case "export":
		return runPackageExport(args[1:], stdout, stderr)
	case "import":
		return runPackageImport(args[1:], home, stdout, stderr)
	default:
		err := fmt.Errorf("unknown package command %q", args[0])
		fmt.Fprintln(stderr, err)
		return err
	}
}

func runPackageImport(args []string, home string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		err := fmt.Errorf("usage: lineage package import <file.tgz> [--as name]")
		fmt.Fprintln(stderr, err)
		return err
	}

	archivePath := args[0]
	asName := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--as" && i+1 < len(args) {
			asName = args[i+1]
			i++
			continue
		}
		err := fmt.Errorf("usage: lineage package import <file.tgz> [--as name]")
		fmt.Fprintln(stderr, err)
		return err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	defer f.Close()

	destParent := config.UserPackagesDir(home)
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	name, err := packages.Import(f, destParent, asName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "imported package %s into %s\n", name, filepath.Join(destParent, name))
	return nil
}

func runPackageExport(args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		err := fmt.Errorf("usage: lineage package export <path> [-o file.tgz]")
		fmt.Fprintln(stderr, err)
		return err
	}

	dir := filepath.Clean(args[0])
	outPath := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			outPath = args[i+1]
			i++
			continue
		}
		err := fmt.Errorf("usage: lineage package export <path> [-o file.tgz]")
		fmt.Fprintln(stderr, err)
		return err
	}

	if outPath == "" {
		manifest, err := packages.LoadManifest(dir)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		outPath = fmt.Sprintf("%s-%s.tgz", manifest.Name, manifest.Version)
	}

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if err := packages.Export(dir, f); err != nil {
		f.Close()
		os.Remove(outPath)
		fmt.Fprintln(stderr, err)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(outPath)
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "exported %s to %s\n", dir, outPath)
	return nil
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

func runList(home string, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	found, err := config.FindProjectConfig(cwd)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if len(found.Config.EnabledPackages) == 0 {
		fmt.Fprintln(stdout, "no packages enabled")
		return nil
	}

	resolved, err := packages.ResolveEnabled(home, found.Config.Workspace, found.Root, found.Config.EnabledPackages)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	for _, pkg := range resolved {
		fmt.Fprintf(stdout, "%s@%s  %s\n", pkg.Manifest.Name, pkg.Manifest.Version, pkg.Digest)
	}
	return nil
}

func runDisable(args []string, home string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		err := fmt.Errorf("usage: lineage disable <package-path-or-id>")
		fmt.Fprintln(stderr, err)
		return err
	}
	ref := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	found, err := config.FindProjectConfig(cwd)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if !contains(found.Config.EnabledPackages, ref) {
		err := fmt.Errorf("package %q is not enabled in %s", ref, found.Path)
		fmt.Fprintln(stderr, err)
		return err
	}

	newEnabled := make([]string, 0, len(found.Config.EnabledPackages)-1)
	for _, r := range found.Config.EnabledPackages {
		if r != ref {
			newEnabled = append(newEnabled, r)
		}
	}

	resolved, err := packages.ResolveEnabled(home, found.Config.Workspace, found.Root, newEnabled)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Re-materialize for every provider that has ever been materialized in
	// this project, so disabling actually removes what was staged - Apply
	// already reconciles to exactly the package set it's given, removing
	// anything no longer desired. A provider that was never materialized
	// here doesn't get a state file created just because something else
	// was disabled.
	for _, p := range provider.Known() {
		hasState, err := materialize.HasState(found.Root, p.Name)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		if !hasState {
			continue
		}
		if err := materialize.Apply(found.Root, p, resolved); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
	}

	found.Config.EnabledPackages = newEnabled
	if err := config.SaveProjectConfig(found.Path, found.Config); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	fmt.Fprintf(stdout, "disabled package %s in %s\n", ref, found.Path)
	return nil
}

func runInspect(args []string, home string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		err := fmt.Errorf("usage: lineage inspect <package-path-or-id>")
		fmt.Fprintln(stderr, err)
		return err
	}
	ref := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Same resolution rule as enable: a "."/absolute ref is relative to
	// where the user typed it, a bare id is looked up against the
	// project/user/workspace search path anchored at the project root (if
	// any - inspect works even outside a project, e.g. against a bare
	// local path).
	projectRoot := cwd
	workspace := ""
	if found, err := config.FindProjectConfig(cwd); err == nil {
		projectRoot = found.Root
		workspace = found.Config.Workspace
	} else if !errors.Is(err, config.ErrProjectConfigNotFound) {
		fmt.Fprintln(stderr, err)
		return err
	}

	resolveRoot := projectRoot
	if filepath.IsAbs(ref) || strings.HasPrefix(ref, ".") {
		resolveRoot = cwd
	}
	path, err := packages.ResolveReference(home, workspace, resolveRoot, ref)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	pkg, err := packages.Discover(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "package: %s@%s (schema %d)\n", pkg.Manifest.Name, pkg.Manifest.Version, pkg.Manifest.Schema)
	fmt.Fprintf(stdout, "path: %s\n", pkg.Path)
	fmt.Fprintf(stdout, "digest: %s\n", emptyValue(pkg.Digest))
	if pkg.Manifest.Description != "" {
		fmt.Fprintf(stdout, "description: %s\n", pkg.Manifest.Description)
	}
	fmt.Fprintf(stdout, "skills: %s\n", listValue(pkg.Skills))
	fmt.Fprintf(stdout, "workflows: %s\n", listValue(pkg.Workflows))
	fmt.Fprintf(stdout, "agents: %s\n", listValue(pkg.Agents))
	fmt.Fprintf(stdout, "policies: %s\n", listValue(pkg.Policies))
	fmt.Fprintf(stdout, "requires.skills: %s\n", listValue(pkg.Manifest.Requires.Skills))
	fmt.Fprintf(stdout, "capabilities:\n")
	fmt.Fprintf(stdout, "  filesystem.read: %s\n", listValue(pkg.Manifest.Capabilities.Filesystem.Read))
	fmt.Fprintf(stdout, "  network: %s\n", listValue(pkg.Manifest.Capabilities.Network))
	return nil
}

func runProvider(ctx context.Context, args []string, home string, stdin io.Reader, stdout, stderr io.Writer) error {
	_ = ctx
	if len(args) == 0 {
		err := fmt.Errorf("usage: lineage run <%s> [--dry-run] [--yes] [-- provider args...]", providerNameList())
		fmt.Fprintln(stderr, err)
		return err
	}

	providerName := args[0]
	dryRun, autoApprove, providerArgs := parseRunArgs(args[1:])

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

	adapter, err := provider.Get(providerName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Materialization writes files into the receiver's project (staged
	// skills, a generated section in the provider's context file). Gate
	// that write behind an explicit confirmation the first time it would
	// change anything, so it's never a surprise side effect of `lineage
	// run` - similar in spirit to how --dry-run already previews the
	// launch plan. Once a given package set is approved and materialized,
	// re-running with the same set doesn't need to ask again.
	needsApproval, err := materialize.NeedsApproval(plan.ProjectRoot, adapter, plan.Packages)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if needsApproval && !autoApprove {
		fmt.Fprintf(stdout, "lineage will make the following changes for %s:\n\n", providerName)
		fmt.Fprint(stdout, plan.DryRunString())
		fmt.Fprint(stdout, "\nProceed? [y/N] ")
		if !confirm(stdin) {
			fmt.Fprintln(stdout, "aborted: materialization was not approved")
			return nil
		}
	}

	if err := materialize.Apply(plan.ProjectRoot, adapter, plan.Packages); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if err := provider.Launch(plan.ProviderPlan); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	return nil
}

// confirm reads a single line from stdin and reports whether it's an
// affirmative answer ("y" or "yes", case-insensitive). Anything else,
// including EOF or a nil reader, is treated as a decline - approval must be
// explicit.
func confirm(stdin io.Reader) bool {
	if stdin == nil {
		return false
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
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

// runDoctor sanity-checks a Lineage setup: project config validity, shim
// PATH placement, and provider binary resolution. It fails (non-zero exit)
// only for things that are actually broken (a project config that doesn't
// parse, an enabled ref that no longer resolves); everything else that's
// merely ambiguous-but-working (multiple provider binary candidates, a shim
// directory not on PATH) is a warning, printed but not fatal.
func runDoctor(home string, stdout, stderr io.Writer) error {
	broken := false

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if found, err := config.FindProjectConfig(cwd); err == nil {
		fmt.Fprintf(stdout, "project config: OK (%s)\n", found.Path)
		if _, err := packages.ResolveEnabled(home, found.Config.Workspace, found.Root, found.Config.EnabledPackages); err != nil {
			fmt.Fprintf(stdout, "enabled packages: FAIL - %v\n", err)
			broken = true
		} else {
			fmt.Fprintf(stdout, "enabled packages: OK (%d enabled)\n", len(found.Config.EnabledPackages))
		}
	} else if errors.Is(err, config.ErrProjectConfigNotFound) {
		fmt.Fprintln(stdout, "project config: not inside a Lineage project (skipping enabled-package checks)")
	} else {
		fmt.Fprintf(stdout, "project config: FAIL - %v\n", err)
		broken = true
	}

	shimsDir := config.ShimsDir(home)
	pathEntries := filepath.SplitList(os.Getenv("PATH"))
	shimIdx := pathIndexOf(pathEntries, shimsDir)
	if shimIdx == -1 {
		fmt.Fprintf(stdout, "shim PATH: WARN - %s is not on PATH; run `lineage install-shims`, then add that directory to PATH before your provider binaries\n", shimsDir)
	} else {
		fmt.Fprintf(stdout, "shim PATH: OK (%s is on PATH)\n", shimsDir)
	}

	for _, p := range provider.Known() {
		candidates := provider.CandidateBinaries(p.Name, home)
		switch len(candidates) {
		case 0:
			fmt.Fprintf(stdout, "provider %s: no real binary found on PATH (not installed, or set providers.%s.binary in .lineage/config.yaml)\n", p.Name, p.Name)
			continue
		case 1:
			fmt.Fprintf(stdout, "provider %s: OK (%s)\n", p.Name, candidates[0])
		default:
			fmt.Fprintf(stdout, "provider %s: WARN - multiple candidates on PATH; the first one wins silently:\n", p.Name)
			for _, c := range candidates {
				fmt.Fprintf(stdout, "    %s\n", c)
			}
		}

		if shimIdx == -1 {
			continue
		}
		winnerIdx := pathIndexOf(pathEntries, filepath.Dir(candidates[0]))
		if winnerIdx != -1 && winnerIdx < shimIdx {
			fmt.Fprintf(stdout, "provider %s: WARN - the real binary at %s comes before the shim directory on PATH, so the shim never takes effect for this provider\n", p.Name, candidates[0])
		}
	}

	if broken {
		err := fmt.Errorf("lineage doctor found problems that need fixing")
		fmt.Fprintln(stderr, err)
		return err
	}
	fmt.Fprintln(stdout, "\nresult: OK")
	return nil
}

// pathIndexOf returns the index of dir within pathEntries (comparing
// absolute paths, so relative and absolute forms of the same directory
// match), or -1 if it isn't present.
func pathIndexOf(pathEntries []string, dir string) int {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return -1
	}
	for i, entry := range pathEntries {
		absEntry, err := filepath.Abs(entry)
		if err == nil && absEntry == absDir {
			return i
		}
	}
	return -1
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(fmt.Sprintf(`
Lineage shareable agent runtime

Usage:
  lineage init user
  lineage init workspace <name>
  lineage package init <name>
  lineage package validate <path>
  lineage package export <path> [-o file.tgz]
  lineage package import <file.tgz> [--as name]
  lineage enable <package-path-or-id>
  lineage disable <package-path-or-id>
  lineage list
  lineage inspect <package-path-or-id>
  lineage run <%s> [--dry-run] [--yes] [-- provider args...]
  lineage install-shims
  lineage doctor
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
// lineage's own --dry-run/--yes flags and the args that should be forwarded
// to the wrapped provider. A bare "--" marks the boundary explicitly:
// everything before it is scanned for lineage flags, everything after it is
// passed through to the provider verbatim, even if it happens to collide
// with a lineage flag name (for example a provider's own --dry-run flag).
// Without "--", the whole argument list is scanned for lineage flags,
// matching the simple common case of `lineage run claude --dry-run`.
func parseRunArgs(args []string) (dryRun, autoApprove bool, providerArgs []string) {
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
		if arg == "--yes" || arg == "-y" {
			autoApprove = true
			continue
		}
		providerArgs = append(providerArgs, arg)
	}
	providerArgs = append(providerArgs, passthrough...)
	return dryRun, autoApprove, providerArgs
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
