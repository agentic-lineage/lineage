package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/agentic-lineage/lineage/internal/auth"
	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/graph"
	"github.com/agentic-lineage/lineage/internal/materialize"
	"github.com/agentic-lineage/lineage/internal/packages"
	"github.com/agentic-lineage/lineage/internal/provider"
	"github.com/agentic-lineage/lineage/internal/runtime"
	"github.com/agentic-lineage/lineage/internal/shim"
	"github.com/agentic-lineage/lineage/internal/snapshot"
)

// Version is set at build time via
// -ldflags "-X github.com/agentic-lineage/lineage/internal/cli.Version=vX.Y.Z"
// (see the release build steps). Defaults to "dev" for a plain `go build`/
// `go run`/`go install` without that flag, e.g. a local development build.
var Version = "dev"

// resolveGitHubToken finds the GitHub token that identifies a publisher:
// LINEAGE_PUBLISH_TOKEN first (the escape hatch for non-interactive use,
// e.g. CI - set it to any GitHub-issued token with read:user access), then
// whatever `lineage login` stored locally. Returns "" with no error if
// neither is set; callers decide how to report that (runWhoAmI and
// runPackagePublish both need it, with different error messages).
func resolveGitHubToken(home string) (string, error) {
	if token := os.Getenv("LINEAGE_PUBLISH_TOKEN"); token != "" {
		return token, nil
	}
	return auth.LoadToken(home)
}

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

	// Wrapped once here, then threaded through as *bufio.Reader rather than
	// io.Reader, so every confirm() prompt within this single invocation
	// reads from the same buffer. A command that prompts more than once
	// (e.g. `add`, which can ask about both enabling and setup) would
	// otherwise have each confirm() call construct its own bufio.Reader,
	// silently discarding whatever that call buffered but didn't consume.
	var in *bufio.Reader
	if stdin != nil {
		in = bufio.NewReader(stdin)
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], home, stdout, stderr)
	case "package":
		return runPackage(args[1:], home, stdout, stderr)
	case "add":
		return runAdd(args[1:], home, in, stdout, stderr)
	case "enable":
		return runEnable(args[1:], home, in, stdout, stderr)
	case "list":
		return runList(home, stdout, stderr)
	case "disable":
		return runDisable(args[1:], home, stdout, stderr)
	case "inspect":
		return runInspect(args[1:], home, stdout, stderr)
	case "graph":
		return runGraph(args[1:], stdout, stderr)
	case "run":
		return runProvider(ctx, args[1:], home, in, stdout, stderr)
	case "workflow":
		return runWorkflow(args[1:], home, in, stdout, stderr)
	case "install-shims":
		return runInstallShims(home, stdout, stderr)
	case "doctor":
		return runDoctor(home, stdout, stderr)
	case "login":
		return runLogin(home, stdout, stderr)
	case "logout":
		return runLogout(home, stdout, stderr)
	case "whoami":
		return runWhoAmI(home, stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "lineage %s\n", Version)
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

// packageUsage is shown for `lineage package` with no subcommand, and for
// `lineage package -h`/`--help`. Each subcommand also answers its own
// `-h`/`--help` with a one-line usage plus a short description - see
// isHelpFlag's call sites below.
const packageUsage = `usage: lineage package <command> [arguments]

Commands:
  init <name>                     scaffold a new package
  validate <path>                 check schema, safety, and exports
  export <path> [-o file.tgz]     write a deterministic archive
  import <file.tgz> [--as name]   install an archive locally
  publish <path>                  publish to the Lineage registry
  pull <ref> [--as name]          fetch a published package

Run 'lineage package <command> --help' for details on one command.`

// isHelpFlag reports whether s is a bare -h/--help flag, so a subcommand
// can answer "-h" with its own usage instead of trying to use it as a
// path/name argument (which would otherwise fail with a confusing
// filesystem or validation error).
func isHelpFlag(s string) bool {
	return s == "-h" || s == "--help"
}

// is there a help flag anywhere in this command's arguments?
func hasHelpFlag(args []string) bool {
	return slices.ContainsFunc(args, isHelpFlag)
}

func runPackage(args []string, home string, stdout, stderr io.Writer) error {
	//   Can't call hasHelpFlag here, otherwise general lineage package help
	// would be printed
	if len(args) > 0 && isHelpFlag(args[0]) {
		fmt.Fprintln(stdout, packageUsage)
		return nil
	}
	if len(args) < 2 {
		err := fmt.Errorf("usage: lineage package init <name> | lineage package validate <path> | lineage package export <path> [-o file.tgz] | lineage package import <file.tgz> [--as name] | lineage package publish <path> | lineage package pull <package-ref> [--as name]")
		fmt.Fprintln(stderr, err)
		return err
	}

	switch args[0] {
	case "init":
		if hasHelpFlag(args[1:]) {
			fmt.Fprintln(stdout, "usage: lineage package init <name>\n\nScaffold a new package: lineage.yaml plus the standard skills/workflows/agents/policies/references/adapters folders.")
			return nil
		}
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
		if hasHelpFlag(args[1:]) {
			fmt.Fprintln(stdout, "usage: lineage package validate <path> [--yaml]\n\nCheck manifest schema, export authority, entrypoint path safety, and scan for secrets - without enabling or writing anything. --yaml prints a stable, provider-independent structured report instead of the human-readable summary.")
			return nil
		}
		path := ""
		yamlOutput := false
		for _, a := range args[1:] {
			if a == "--yaml" {
				yamlOutput = true
				continue
			}
			if path != "" {
				err := fmt.Errorf("usage: lineage package validate <path> [--yaml]")
				fmt.Fprintln(stderr, err)
				return err
			}
			path = a
		}
		if path == "" {
			err := fmt.Errorf("usage: lineage package validate <path> [--yaml]")
			fmt.Fprintln(stderr, err)
			return err
		}
		return runPackageValidate(filepath.Clean(path), yamlOutput, stdout, stderr)
	case "export":
		return runPackageExport(args[1:], stdout, stderr)
	case "import":
		return runPackageImport(args[1:], home, stdout, stderr)
	case "publish":
		return runPackagePublish(args[1:], home, stdout, stderr)
	case "pull":
		return runPackagePull(args[1:], home, stdout, stderr)
	default:
		err := fmt.Errorf("unknown package command %q", args[0])
		fmt.Fprintln(stderr, err)
		return err
	}
}

func runPackagePublish(args []string, home string, stdout, stderr io.Writer) error {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage package publish <path>\n\nValidate and publish a package to the Lineage registry, identified by the GitHub login from `lineage login` (or LINEAGE_PUBLISH_TOKEN). The first publish of a name claims it; later publishes need the same identity.")
		return nil
	}
	if len(args) != 1 {
		err := fmt.Errorf("usage: lineage package publish <path>")
		fmt.Fprintln(stderr, err)
		return err
	}

	token, err := resolveGitHubToken(home)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if token == "" {
		err := fmt.Errorf("not logged in; run `lineage login` first, or set LINEAGE_PUBLISH_TOKEN for non-interactive use")
		fmt.Fprintln(stderr, err)
		return err
	}

	cfg := packages.RegistryConfig{URL: os.Getenv("LINEAGE_REGISTRY_URL"), Token: token}
	result, err := packages.Publish(filepath.Clean(args[0]), cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if result.AlreadyPublished {
		fmt.Fprintf(stdout, "%s@%s is already published with this content (digest %s); nothing to do\n", result.Name, result.Version, result.Digest)
		return nil
	}
	fmt.Fprintf(stdout, "published %s@%s (digest %s)\n", result.Name, result.Version, result.Digest)
	return nil
}

func runPackagePull(args []string, home string, stdout, stderr io.Writer) error {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage package pull <package-ref> [--as name]\n\nFetch a published package (ref is \"name\" for the latest version, or an exact \"name@version\") and verify its content digest. An unauthenticated read - no login needed.")
		return nil
	}
	if len(args) < 1 {
		err := fmt.Errorf("usage: lineage package pull <package-ref> [--as name]")
		fmt.Fprintln(stderr, err)
		return err
	}

	ref := args[0]
	asName := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--as" && i+1 < len(args) {
			asName = args[i+1]
			i++
			continue
		}
		err := fmt.Errorf("usage: lineage package pull <package-ref> [--as name]")
		fmt.Fprintln(stderr, err)
		return err
	}

	destParent := config.UserPackagesDir(home)
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Pull is an unauthenticated read - the registry doesn't gate who can
	// fetch a published package, only who can publish one.
	cfg := packages.RegistryConfig{URL: os.Getenv("LINEAGE_REGISTRY_URL")}
	name, err := packages.Pull(ref, cfg, destParent, asName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "pulled package %s into %s\n", name, filepath.Join(destParent, name))
	return nil
}

func runPackageImport(args []string, home string, stdout, stderr io.Writer) error {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage package import <file.tgz> [--as name]\n\nExtract an exported archive into the user packages directory, re-validating it as untrusted input. Never overwrites an existing package; use --as to import under a different name.")
		return nil
	}
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
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage package export <path> [-o file.tgz]\n\nWrite a deterministic .tgz archive of the package after running the same checks as validate. Refuses to export a package that fails validation.")
		return nil
	}
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

	report, err := packages.Validate(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	portability := packages.NewPortabilityReport(report)
	packages.WritePortabilityReport(stdout, portability)
	if portability.HasBlockers() {
		err := fmt.Errorf("package has unresolved portability blockers, refusing to export (%d blocker(s)); run `lineage package validate` for details", len(portability.Blockers))
		fmt.Fprintln(stderr, err)
		return err
	}

	if outPath == "" {
		outPath = fmt.Sprintf("%s-%s.tgz", report.Manifest.Name, report.Manifest.Version)
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

func runPackageValidate(dir string, yamlOutput bool, stdout, stderr io.Writer) error {
	report, err := packages.Validate(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if yamlOutput {
		// Best-effort: a package failing validation in a way that also
		// breaks discovery (a traversing entrypoint, a declared-but-missing
		// export) won't have one, and validateReport omits
		// skills/workflows/agents/policies rather than guessing - the
		// report's own errors already explain why.
		discovered, discoverErr := packages.Discover(dir)
		var discoveredPtr *packages.Package
		if discoverErr == nil {
			discoveredPtr = &discovered
		}
		if writeErr := writeYAML(stdout, validateReport(report, discoveredPtr)); writeErr != nil {
			fmt.Fprintln(stderr, writeErr)
			return writeErr
		}
		if !report.Passed() {
			err := fmt.Errorf("package validation failed with %d error(s)", len(report.Errors))
			fmt.Fprintln(stderr, err)
			return err
		}
		return nil
	}

	fmt.Fprintf(stdout, "package: %s@%s (schema %d)\n", report.Manifest.Name, report.Manifest.Version, report.Manifest.Schema)
	fmt.Fprintf(stdout, "digest: %s\n", emptyValue(report.Digest))
	fmt.Fprintf(stdout, "capabilities:\n")
	fmt.Fprintf(stdout, "  filesystem.read: %s\n", listValue(report.Manifest.Capabilities.Filesystem.Read))
	fmt.Fprintf(stdout, "  network: %s\n", listValue(report.Manifest.Capabilities.Network))
	packages.WritePortabilityReport(stdout, packages.NewPortabilityReport(report))

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

// describeSuffix formats a setup item's optional description for the
// setup plan printout: " - <description>" if present, otherwise nothing.
func describeSuffix(description string) string {
	if description == "" {
		return ""
	}
	return " - " + description
}

func runEnable(args []string, home string, stdin *bufio.Reader, stdout, stderr io.Writer) error {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage enable <package-path-or-id> [--yes]\n\nEnable a package in the current project. If the package declares setup - tracker files or directories its workflow expects - shows the plan and asks permission before creating anything; --yes skips that prompt.")
		return nil
	}
	ref := ""
	autoApprove := false
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			autoApprove = true
			continue
		}
		if ref != "" {
			err := fmt.Errorf("usage: lineage enable <package-path-or-id> [--yes]")
			fmt.Fprintln(stderr, err)
			return err
		}
		ref = a
	}
	if ref == "" {
		err := fmt.Errorf("usage: lineage enable <package-path-or-id> [--yes]")
		fmt.Fprintln(stderr, err)
		return err
	}
	return enableRef(ref, home, autoApprove, stdin, stdout, stderr)
}

// enableRef is runEnable's actual work, factored out so `lineage add`
// (which pulls a package and then enables it in one command) shares the
// exact same project-root resolution, dependency validation, and setup
// handling instead of a second, drifting implementation.
func enableRef(ref, home string, autoApprove bool, stdin *bufio.Reader, stdout, stderr io.Writer) error {
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

	// A package can declare local setup (#72) - tracker files or
	// directories its workflow expects to exist. Show exactly what would
	// be created and get permission before writing anything; declining
	// leaves the workspace completely unchanged, same as declining to
	// enable at all, rather than enabling into a half-set-up state.
	manifest, err := packages.LoadManifest(resolvedPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	setupPlan, err := packages.PlanSetup(projectRoot, manifest.Setup)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	if setupPlan.NeedsAction() {
		fmt.Fprintf(stdout, "%s wants to set up:\n", manifest.Name)
		for _, f := range setupPlan.Files {
			if f.Exists {
				continue
			}
			fmt.Fprintf(stdout, "  create file %s%s\n", f.Path, describeSuffix(f.Description))
		}
		for _, d := range setupPlan.Directories {
			if d.Exists {
				continue
			}
			fmt.Fprintf(stdout, "  create directory %s%s\n", d.Path, describeSuffix(d.Description))
		}
		if !autoApprove {
			fmt.Fprint(stdout, "Create these? [y/N] ")
			if !confirm(stdin) {
				fmt.Fprintln(stdout, "not enabled - setup was declined. Nothing was created or changed.")
				return nil
			}
		}
		if err := packages.ApplySetup(projectRoot, setupPlan); err != nil {
			fmt.Fprintln(stderr, err)
			return err
		}
		fmt.Fprintln(stdout, "setup complete.")
	}

	cfg.EnabledPackages = newEnabled
	if err := config.SaveProjectConfig(configPath, cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Record that this project's local environment now descends from
	// manifest, so `lineage graph list` can later explain where it came
	// from (#6). Digest is recomputed here (ComputeDigest, the same
	// identity Pull/Import verify against) rather than reusing a value
	// from earlier resolution, since enableRef never needed it until now.
	digest, err := packages.ComputeDigest(resolvedPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	// Also take a durable, content-addressed snapshot of exactly what was
	// enabled (#7), so the graph record above can point at something that
	// can later be inspected, copied, or reconstructed byte-for-byte
	// instead of only naming a version.
	_, snapshotID, err := snapshot.Create(home, resolvedPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	if _, err := graph.Append(projectRoot, graph.Record{
		Event: "enable",
		Parent: graph.ParentRef{
			Name:       manifest.Name,
			Version:    manifest.Version,
			Digest:     digest,
			SnapshotID: string(snapshotID),
		},
		Descendant: graph.DescendantRef{
			Workspace: cfg.Workspace,
		},
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "enabled package %s in %s\n", storedRef, configPath)
	return nil
}

// runAdd is the one-command receiver path (#77): pull a published package,
// show what it contains, ask permission, then enable it - fetch, inspect,
// and enable in one step instead of three separate commands. This is what
// the bootstrap prompt (#98) and `lineage add`'s own direct users both
// build on.
// importAddSource brings ref into destParent as an extracted, digest-
// verified package directory, converging local .tgz archives and registry
// refs on the exact same shape before the shared inspect -> confirm ->
// enable pipeline in runAdd ever runs (#71) - nothing past this point
// cares which kind of source a package came from. action names what
// actually happened ("fetched", "imported", or "already have") so runAdd
// can report it accurately instead of always claiming to have fetched.
//
// Re-running add for a package already present locally is a deliberate
// no-op, not a failure: Import/Pull both refuse to overwrite an existing
// package directory, so a second run would otherwise error even though
// nothing is wrong. This treats that specific error as success as long as
// the content matches (digests equal) - a genuine conflict, the same
// name/version now resolving to different content, still fails loudly
// instead of silently keeping the stale local copy.
func importAddSource(ref, destParent string) (name, action string, err error) {
	if info, statErr := os.Stat(ref); statErr == nil && !info.IsDir() {
		f, openErr := os.Open(ref)
		if openErr != nil {
			return "", "", openErr
		}
		defer f.Close()
		name, err = packages.Import(f, destParent, "")
		action = "imported"
	} else {
		cfg := packages.RegistryConfig{URL: os.Getenv("LINEAGE_REGISTRY_URL")}
		name, err = packages.Pull(ref, cfg, destParent, "")
		action = "fetched"
	}

	var already *packages.ErrAlreadyImported
	if errors.As(err, &already) {
		existingDigest, digestErr := packages.ComputeDigest(already.Dest)
		if digestErr == nil && existingDigest == already.Digest {
			return already.Name, "already have", nil
		}
		if digestErr == nil {
			return "", "", fmt.Errorf("package %q already exists locally with different content (local digest %s, requested digest %s); remove %s first if you want to replace it", already.Name, existingDigest, already.Digest, already.Dest)
		}
	}
	return name, action, err
}

func runAdd(args []string, home string, stdin *bufio.Reader, stdout, stderr io.Writer) error {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage add <package-ref> [--yes]\n\nFetch a published package, show what it contains, ask permission, then enable it in the current project - the one-command receiver path. <package-ref> is a registry ref (name or name@version), or the path to a local .tgz archive from `lineage package export`.")
		return nil
	}
	if len(args) < 1 {
		err := fmt.Errorf("usage: lineage add <package-ref> [--yes]")
		fmt.Fprintln(stderr, err)
		return err
	}

	ref := args[0]
	autoApprove := false
	for _, a := range args[1:] {
		if a == "--yes" || a == "-y" {
			autoApprove = true
			continue
		}
		err := fmt.Errorf("usage: lineage add <package-ref> [--yes]")
		fmt.Fprintln(stderr, err)
		return err
	}

	destParent := config.UserPackagesDir(home)
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	name, action, err := importAddSource(ref, destParent)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	pkg, err := packages.Discover(filepath.Join(destParent, name))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}

	fmt.Fprintf(stdout, "%s %s@%s\n", action, pkg.Manifest.Name, pkg.Manifest.Version)
	fmt.Fprintf(stdout, "digest: %s\n", emptyValue(pkg.Digest))
	if pkg.Manifest.Description != "" {
		fmt.Fprintf(stdout, "description: %s\n", pkg.Manifest.Description)
	}
	fmt.Fprintf(stdout, "skills: %s\n", listValue(pkg.Skills))
	fmt.Fprintf(stdout, "workflows: %s\n", listValue(pkg.Workflows))
	fmt.Fprintf(stdout, "agents: %s\n", listValue(pkg.Agents))
	fmt.Fprintf(stdout, "policies: %s\n", listValue(pkg.Policies))
	fmt.Fprintf(stdout, "capabilities:\n")
	fmt.Fprintf(stdout, "  filesystem.read: %s\n", listValue(pkg.Manifest.Capabilities.Filesystem.Read))
	fmt.Fprintf(stdout, "  network: %s\n", listValue(pkg.Manifest.Capabilities.Network))

	if !autoApprove {
		fmt.Fprint(stdout, "\nEnable this package in the current project? [y/N] ")
		if !confirm(stdin) {
			fmt.Fprintf(stdout, "not enabled. It's still available locally - run `lineage enable %s` when you're ready.\n", name)
			return nil
		}
	}

	fmt.Fprintln(stdout)
	if err := enableRef(name, home, autoApprove, stdin, stdout, stderr); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\nReady. Run `lineage run <%s>` to use it.\n", providerNameList())
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
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, "usage: lineage inspect <package-path-or-id> [--yaml]\n\nShow a package's contents. --yaml prints a stable, provider-independent structured report instead of the human-readable summary.")
		return nil
	}
	ref := ""
	yamlOutput := false
	for _, a := range args {
		if a == "--yaml" {
			yamlOutput = true
			continue
		}
		if ref != "" {
			err := fmt.Errorf("usage: lineage inspect <package-path-or-id> [--yaml]")
			fmt.Fprintln(stderr, err)
			return err
		}
		ref = a
	}
	if ref == "" {
		err := fmt.Errorf("usage: lineage inspect <package-path-or-id> [--yaml]")
		fmt.Fprintln(stderr, err)
		return err
	}

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

	if yamlOutput {
		return writeYAML(stdout, inspectReport(pkg))
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

func runProvider(ctx context.Context, args []string, home string, stdin *bufio.Reader, stdout, stderr io.Writer) error {
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

func runWorkflow(args []string, home string, stdin *bufio.Reader, stdout, stderr io.Writer) error {
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
//
// Takes the caller's own *bufio.Reader rather than wrapping an io.Reader
// itself: a command that prompts more than once in one run (e.g. `add`,
// which can ask about both enabling and setup) must read successive lines
// off the same buffer. Constructing a fresh bufio.Reader per call would
// silently discard whatever the previous call had already buffered but not
// consumed if a single Read from stdin returned more than one line's worth
// of bytes at once - the exact shape piped/scripted stdin takes.
func confirm(stdin *bufio.Reader) bool {
	if stdin == nil {
		return false
	}
	line, err := stdin.ReadString('\n')
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
Lineage - package a working agent setup, share it, and run it through your
own Claude or Codex.

Usage:
  lineage <command> [arguments]

Package authoring:
  package init <name>                     scaffold a new package
  package validate <path> [--yaml]         check schema, safety, and exports
  package export <path> [-o file.tgz]     write a deterministic archive
  package import <file.tgz> [--as name]   install an archive locally

Registry:
  add <ref> [--yes]                       fetch, inspect, and enable a package in one step
  login                                   authorize with GitHub (device flow)
  logout                                  clear the stored credential
  whoami                                  show who publish/pull will act as
  package publish <path>                  publish to the Lineage registry
  package pull <ref> [--as name]          fetch a published package

Using a package:
  enable <path-or-id> [--yes]              add a package to this project
  disable <path-or-id>                    remove a package from this project
  list                                    show enabled packages
  inspect <path-or-id> [--yaml]            show a package's contents
  graph list [--yaml]                      show what this project's state descends from
  run <%s> [--dry-run] [--yes]  launch a provider with packages applied
  workflow run <name> <%s>      run one declared workflow

Setup:
  init user                               create the user package directory
  init workspace <name>                   create a shared workspace
  install-shims                           put lineage in front of claude/codex on PATH
  doctor                                  check config, PATH, and provider setup

  -h, --help                              show this help
  -v, --version                           show the CLI version

Run 'lineage package --help' or 'lineage <command> --help' for details on one command.
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
