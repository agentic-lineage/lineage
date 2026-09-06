// Package tests holds higher-level fixtures and integration tests that
// exercise Lineage's CLI commands together, as a receiver actually would,
// rather than one command's internals in isolation.
package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/cli"
	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

// TestReceiverFlow recreates the full distribution story end to end, as one
// fixture instead of the same steps split across many package-level unit
// tests: an author creates and validates a package, exports it to a single
// archive file, and a receiver — a completely separate LINEAGE_HOME and
// project directory, simulating a different machine, sharing nothing with
// the author but the archive file itself — imports it, inspects it,
// enables it, runs the whole package, runs just one of its workflows, then
// lists, disables, and re-checks with doctor. Every step goes through the
// real CLI entrypoint (cli.Execute), so this proves the actual command
// surface works together end to end, not just the underlying library code
// in isolation.
func TestReceiverFlow(t *testing.T) {
	root := t.TempDir()

	// --- Author side: build a small but genuine three-skill package with an
	// ordered workflow, and validate it before it ever leaves this
	// machine. ---
	authorHome := filepath.Join(root, "author-home")
	pkgDir := filepath.Join(root, "field-notes")

	run(t, authorHome, root, nil, "package", "init", pkgDir)

	writeFile(t, filepath.Join(pkgDir, "skills", "collect", "SKILL.md"), "---\nname: collect\ndescription: Collect field notes from the receiver.\n---\n\nAsk the receiver for the essentials before doing anything else.\n")
	writeFile(t, filepath.Join(pkgDir, "skills", "summarize", "SKILL.md"), "---\nname: summarize\ndescription: Summarize collected field notes.\n---\n\nTurn the collected notes into a short, honest summary.\n")
	writeFile(t, filepath.Join(pkgDir, "skills", "archive", "SKILL.md"), "---\nname: archive\ndescription: Archive completed field reports.\n---\n\nStore the final report only after the active workflow is complete.\n")
	writeFile(t, filepath.Join(pkgDir, "workflows", "field-report", "WORKFLOW.md"), "---\nsteps:\n  - collect\n  - summarize\n---\n\n# Field Report\n\n1. Collect notes.\n2. Summarize them.\n")

	validateOut := run(t, authorHome, root, nil, "package", "validate", pkgDir)
	if !strings.Contains(validateOut, "result: PASS") {
		t.Fatalf("package validate = %q, want PASS before it's ever exported", validateOut)
	}

	archivePath := filepath.Join(root, "field-notes.tgz")
	run(t, authorHome, root, nil, "package", "export", pkgDir, "-o", archivePath)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected exported archive: %v", err)
	}

	// --- Receiver side: nothing shared with the author except the
	// archive. A fresh LINEAGE_HOME and a fresh project directory. ---
	receiverHome := filepath.Join(root, "receiver-home")
	receiverProject := filepath.Join(root, "receiver-project")
	mkdir(t, receiverProject)

	importOut := run(t, receiverHome, receiverProject, nil, "package", "import", archivePath)
	if !strings.Contains(importOut, "imported package field-notes") {
		t.Fatalf("import output = %q, want it to name the imported package", importOut)
	}

	importedDir := filepath.Join(config.UserPackagesDir(receiverHome), "field-notes")

	// Inspect before enabling - a receiver should be able to see what
	// they're about to turn on, without it being enabled yet.
	inspectOut := run(t, receiverHome, receiverProject, nil, "inspect", "field-notes")
	if !strings.Contains(inspectOut, "field-notes@0.1.0") || !strings.Contains(inspectOut, "workflows: field-report") {
		t.Fatalf("inspect output = %q, want package identity and its workflow", inspectOut)
	}
	if _, err := os.Stat(config.ProjectConfigPath(receiverProject)); !os.IsNotExist(err) {
		t.Fatal("inspect must not have enabled anything yet")
	}

	// Confirm identity survived the trip intact before doing anything
	// else: the receiver's imported package must have the same
	// content identity as the author's package, not just be "close
	// enough".
	authorPkg, err := packages.Discover(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	receiverPkg, err := packages.Discover(importedDir)
	if err != nil {
		t.Fatal(err)
	}
	if receiverPkg.Digest != authorPkg.Digest {
		t.Fatalf("receiver digest = %q, want %q (exact reconstruction)", receiverPkg.Digest, authorPkg.Digest)
	}

	run(t, receiverHome, receiverProject, nil, "enable", "field-notes")

	// A fake provider binary standing in for a real `claude`/`codex` on
	// the receiver's machine. It records that it ran to a marker file
	// rather than stdout: provider.Launch wires the real process's stdout
	// straight to the OS's own stdout (a real terminal-interactive
	// provider needs that), not to cli.Execute's captured stdout buffer,
	// so a marker file is the reliable way to confirm it actually ran.
	launchMarker := filepath.Join(root, "claude-launched.marker")
	fakeClaude := writeLaunchMarkerProvider(t, filepath.Join(receiverProject, "bin"), launchMarker)
	setProviderBinary(t, receiverProject, "claude", fakeClaude)

	run(t, receiverHome, receiverProject, nil, "run", "claude", "--yes")
	if _, err := os.Stat(launchMarker); err != nil {
		t.Fatalf("expected the provider to have actually launched: %v", err)
	}
	for _, skill := range []string{"collect", "summarize", "archive"} {
		p := filepath.Join(receiverProject, ".claude", "skills", "field-notes-"+skill, "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s materialized: %v", skill, err)
		}
	}
	claudeMD := readFile(t, filepath.Join(receiverProject, "CLAUDE.md"))
	if !strings.Contains(claudeMD, "field-notes@0.1.0") {
		t.Fatalf("CLAUDE.md = %q, want the active package listed", claudeMD)
	}

	// Run just the workflow: scoped materialization should replace the
	// full-package materialization with only the workflow's steps, in
	// order, and say so in the generated context.
	run(t, receiverHome, receiverProject, nil, "workflow", "run", "field-report", "claude", "--yes")
	if got := countLines(t, launchMarker); got != 2 {
		t.Fatalf("provider launched %d times so far, want 2 (full run + workflow run)", got)
	}
	claudeMD = readFile(t, filepath.Join(receiverProject, "CLAUDE.md"))
	if !strings.Contains(claudeMD, "Active workflow: field-report") ||
		!strings.Contains(claudeMD, "1. collect") ||
		!strings.Contains(claudeMD, "2. summarize") {
		t.Fatalf("CLAUDE.md = %q, want the ordered workflow sequence", claudeMD)
	}
	if _, err := os.Stat(filepath.Join(receiverProject, ".claude", "skills", "field-notes-archive")); !os.IsNotExist(err) {
		t.Fatalf("expected archive absent from scoped workflow materialization, stat err = %v", err)
	}

	// A plain `lineage run` afterward must restore the full package -
	// scoping to a workflow is reversible, not a one-way narrowing.
	run(t, receiverHome, receiverProject, nil, "run", "claude", "--yes")
	for _, skill := range []string{"collect", "summarize", "archive"} {
		p := filepath.Join(receiverProject, ".claude", "skills", "field-notes-"+skill)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s restored by the full run: %v", skill, err)
		}
	}

	listOut := run(t, receiverHome, receiverProject, nil, "list")
	if !strings.Contains(listOut, "field-notes@0.1.0") {
		t.Fatalf("list output = %q, want the enabled package", listOut)
	}

	run(t, receiverHome, receiverProject, nil, "disable", "field-notes", "--yes")
	for _, skill := range []string{"collect", "summarize", "archive"} {
		p := filepath.Join(receiverProject, ".claude", "skills", "field-notes-"+skill)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s cleaned up after disable, stat err = %v", skill, err)
		}
	}
	listOut = run(t, receiverHome, receiverProject, nil, "list")
	if !strings.Contains(listOut, "no packages enabled") {
		t.Fatalf("list output after disable = %q, want it empty", listOut)
	}

	doctorOut := run(t, receiverHome, receiverProject, nil, "doctor")
	if !strings.Contains(doctorOut, "result: OK") {
		t.Fatalf("doctor output = %q, want a clean bill of health throughout", doctorOut)
	}
}

// run executes a lineage CLI command with home as LINEAGE_HOME and cwd as
// the working directory, failing the test immediately on error so a
// broken step in the flow points straight at the command that broke it.
func run(t *testing.T, home, cwd string, stdin io.Reader, args ...string) string {
	t.Helper()

	t.Setenv(config.HomeEnv, home)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := cli.Execute(nil, args, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("lineage %s: error = %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func setProviderBinary(t *testing.T, project, providerName, binaryPath string) {
	t.Helper()
	cfgPath := config.ProjectConfigPath(project)
	cfg, err := config.LoadProjectConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]config.Provider{}
	}
	cfg.Providers[providerName] = config.Provider{Binary: binaryPath}
	if err := config.SaveProjectConfig(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLaunchMarkerProvider(t *testing.T, dir, markerPath string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "claude.cmd")
		content := "@echo off\r\n" +
			">> \"" + markerPath + "\" echo launched\r\n"
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}

	path := filepath.Join(dir, "claude")
	content := "#!/bin/sh\n" +
		"echo launched >> \"" + markerPath + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Split(strings.TrimRight(string(data), "\n"), "\n"))
}
