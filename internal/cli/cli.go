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
	case "run":
		return runProvider(ctx, args[1:], home, stdin, stdout, stderr)
	case "workflow":
		return runWorkflow(args[1:], home, stdin, stdout, stderr)
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

func runWorkflow(args []string, home string, stdin io.Reader, stdout, stderr io.Writer) error {
	usage := fmt.Errorf("usage: lineage workflow run <workflow-name> <%s> [--dry-run] [--yes] [-- provider args...]", providerNameList())
	if len(args) < 3 || args[0] != "run" {
		fmt.Fprintln(stderr, usage)
		return usage
	}

	workflowName := args[1]
	providerName := args[2]
	dryRun, autoApprove, providerArgs := parseRunArgs(args[3:])

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

	resolved, err := packages.ResolveEnabled(home, found.Config.Workspace, found.Root, found.Config.EnabledPackages)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if err := packages.ValidateDependencies(resolved); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	ownerPkg, wf, err := packages.FindWorkflow(resolved, workflowName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	adapter, err := provider.Get(providerName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	providerPlan, err := provider.Resolve(providerName, home, found.Config, providerArgs)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if dryRun {
		fmt.Fprint(stdout, workflowPlanString(wf, ownerPkg, providerName, providerPlan))
		return nil
	}

	// Same permission gate as `lineage run`, scoped to just this workflow's
	// steps rather than the whole enabled package set - see
	// materialize.ApplyWorkflow.
	needsApproval, err := materialize.NeedsApprovalForWorkflow(found.Root, adapter, ownerPkg, wf)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if needsApproval && !autoApprove {
		fmt.Fprintf(stdout, "lineage will make the following changes to run workflow %q for %s:\n\n", wf.Name, providerName)
		fmt.Fprint(stdout, workflowPlanString(wf, ownerPkg, providerName, providerPlan))
		fmt.Fprint(stdout, "\nProceed? [y/N] ")
		if !confirm(stdin) {
			fmt.Fprintln(stdout, "aborted: materialization was not approved")
			return nil
		}
	}

	if err := materialize.ApplyWorkflow(found.Root, adapter, ownerPkg, wf); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if err := provider.Launch(providerPlan); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	return nil
}

func workflowPlanString(wf packages.Workflow, pkg packages.Package, providerName string, providerPlan provider.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Lineage workflow plan\n")
	fmt.Fprintf(&b, "workflow: %s\n", wf.Name)
	fmt.Fprintf(&b, "package: %s@%s\n", pkg.Manifest.Name, pkg.Manifest.Version)
	fmt.Fprintf(&b, "provider: %s\n", providerName)
	fmt.Fprintf(&b, "real_binary: %s\n", emptyValue(providerPlan.Binary))
	fmt.Fprintf(&b, "steps:\n")
	for i, step := range wf.Steps {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, step)
	}
	return b.String()
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
  lineage run <%s> [--dry-run] [--yes] [-- provider args...]
  lineage workflow run <workflow-name> <%s> [--dry-run] [--yes] [-- provider args...]
  lineage install-shims
`, providerNameList(), providerNameList())))
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
