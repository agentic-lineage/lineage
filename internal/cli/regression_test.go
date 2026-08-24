package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

// TestEnableFromSubdirectoryUsesProjectRoot guards against a regression
// where `lineage enable` used cwd directly instead of walking up to find an
// already-initialized project, the same way `lineage run` does. Enabling a
// package from any subdirectory of a project must update that project's
// existing root config (not fork a second, shadow .lineage/config.yaml),
// and a relative ref must be re-expressed relative to the project root so
// `lineage run` (which resolves enabled_packages against the root) still
// finds the exact directory the user pointed at.
func TestEnableFromSubdirectoryUsesProjectRoot(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	sub := filepath.Join(project, "packages", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Project already has an initialized root config (e.g. from a previous
	// `lineage enable` at the root, or just `lineage init`).
	rootConfigPath := config.ProjectConfigPath(project)
	if err := config.SaveProjectConfig(rootConfigPath, config.DefaultProjectConfig()); err != nil {
		t.Fatal(err)
	}

	// A package that happens to live under the subdirectory.
	pkgDir := filepath.Join(sub, "local-pack")
	if err := packages.InitPackage(pkgDir, "local-pack"); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./local-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	// No shadow config should have been created in the subdirectory.
	if _, err := os.Stat(config.ProjectConfigPath(sub)); err == nil {
		t.Fatalf("a second, shadow .lineage/config.yaml was created at %s; `lineage enable` should reuse the project root config discovered via FindProjectConfig, the same way `lineage run` does", config.ProjectConfigPath(sub))
	}

	// The ROOT config should list the package, re-expressed relative to the
	// project root (not the literal "./local-pack" the user typed, which
	// would resolve to the wrong directory from the root).
	rootCfg, err := config.LoadProjectConfig(rootConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	wantRef := "./packages/sub/local-pack"
	if !contains(rootCfg.EnabledPackages, wantRef) {
		t.Fatalf("root config EnabledPackages = %#v; want it to contain %q", rootCfg.EnabledPackages, wantRef)
	}

	// And it must actually resolve back to the package the user pointed at,
	// the way `lineage run` will resolve it (against the project root).
	for _, storedRef := range rootCfg.EnabledPackages {
		resolved, err := packages.ResolveReference(home, rootCfg.Workspace, project, storedRef)
		if err != nil {
			t.Fatalf("ResolveReference(%q) error = %v", storedRef, err)
		}
		if resolved != pkgDir {
			t.Fatalf("ResolveReference(%q) = %q, want %q", storedRef, resolved, pkgDir)
		}
	}
}

// TestEnableFromProjectRootIsUnaffected guards against a regression in the
// common case (enabling from the project root itself): the ref should be
// stored exactly as typed, matching the pre-existing documented usage
// (`lineage enable ./resume-workflow`).
func TestEnableFromProjectRootIsUnaffected(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "agent-pack")
	if err := packages.InitPackage(pkgDir, "agent-pack"); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	cfg, err := config.LoadProjectConfig(config.ProjectConfigPath(project))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(cfg.EnabledPackages, "./agent-pack") {
		t.Fatalf("EnabledPackages = %#v, want it to contain ./agent-pack unchanged", cfg.EnabledPackages)
	}
}

// TestRunPassesFlagsAfterDoubleDashToProvider guards against a regression
// where arguments after the `--` separator were still scanned for
// lineage's own flag names. Everything after `--` belongs to the wrapped
// provider and must be passed through verbatim, even if it collides with a
// lineage-level flag such as --dry-run.
func TestRunPassesFlagsAfterDoubleDashToProvider(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	// Fake provider binary that records the args it was invoked with.
	marker := filepath.Join(tmp, "invoked-args.txt")
	fakeBinary := filepath.Join(tmp, "fake-claude.sh")
	script := "#!/bin/sh\necho \"$@\" > " + marker + "\n"
	if err := os.WriteFile(fakeBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultProjectConfig()
	cfg.Providers = map[string]config.Provider{"claude": {Binary: fakeBinary}}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), cfg); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	// Everything after "--" is meant for the provider, including a literal
	// "--dry-run" flag that the wrapped agent itself understands.
	if err := Execute(nil, []string{"run", "claude", "--", "--dry-run"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run error = %v stderr=%s", err, stderr.String())
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("provider binary was never invoked (lineage likely intercepted --dry-run as its own flag instead of passing it through): %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "--dry-run" {
		t.Fatalf("provider invoked with args %q, want %q", got, "--dry-run")
	}
}

// TestParseRunArgs exercises the lineage-flag / provider-arg split directly,
// covering both the no-"--" shorthand and the explicit separator form.
func TestParseRunArgs(t *testing.T) {
	cases := []struct {
		name            string
		args            []string
		wantDryRun      bool
		wantAutoApprove bool
		wantForward     []string
	}{
		{
			name:        "dry-run without separator",
			args:        []string{"--dry-run"},
			wantDryRun:  true,
			wantForward: []string{},
		},
		{
			name:            "yes without separator",
			args:            []string{"--yes"},
			wantAutoApprove: true,
			wantForward:     []string{},
		},
		{
			name:            "short -y flag",
			args:            []string{"-y"},
			wantAutoApprove: true,
			wantForward:     []string{},
		},
		{
			name:            "provider's own --yes after separator is preserved",
			args:            []string{"--", "--yes"},
			wantAutoApprove: false,
			wantForward:     []string{"--yes"},
		},
		{
			name:        "plain args without separator",
			args:        []string{"--model", "sonnet"},
			wantDryRun:  false,
			wantForward: []string{"--model", "sonnet"},
		},
		{
			name:        "dry-run before separator, passthrough after",
			args:        []string{"--dry-run", "--", "--model", "sonnet"},
			wantDryRun:  true,
			wantForward: []string{"--model", "sonnet"},
		},
		{
			name:        "provider's own --dry-run after separator is preserved",
			args:        []string{"--model", "sonnet", "--", "--dry-run"},
			wantDryRun:  false,
			wantForward: []string{"--model", "sonnet", "--dry-run"},
		},
		{
			name:        "bare separator with nothing after",
			args:        []string{"--dry-run", "--"},
			wantDryRun:  true,
			wantForward: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dryRun, autoApprove, forward := parseRunArgs(tc.args)
			if dryRun != tc.wantDryRun {
				t.Fatalf("dryRun = %v, want %v", dryRun, tc.wantDryRun)
			}
			if autoApprove != tc.wantAutoApprove {
				t.Fatalf("autoApprove = %v, want %v", autoApprove, tc.wantAutoApprove)
			}
			if len(forward) != len(tc.wantForward) {
				t.Fatalf("forward = %#v, want %#v", forward, tc.wantForward)
			}
			for i := range forward {
				if forward[i] != tc.wantForward[i] {
					t.Fatalf("forward = %#v, want %#v", forward, tc.wantForward)
				}
			}
		})
	}
}

// TestInitWorkspaceRejectsTraversingName covers #171: `lineage init
// workspace ../../escaped` created `.../escaped/packages` outside the
// intended workspaces/ directory, because the name went straight into
// filepath.Join unvalidated.
func TestInitWorkspaceRejectsTraversingName(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home", "lineage")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.HomeEnv, home)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"init", "workspace", filepath.Join("..", "..", "escaped")}, nil, &stdout, &stderr); err == nil {
		t.Fatal("init workspace with a traversing name: error = nil, want a rejection")
	}

	escaped := filepath.Join(tmp, "escaped")
	if _, err := os.Stat(escaped); err == nil {
		t.Fatalf("a workspace directory was created at %s, outside %s", escaped, config.WorkspacesDir(home))
	}

	// A well-formed name still works.
	stdout.Reset()
	stderr.Reset()
	if err := Execute(nil, []string{"init", "workspace", "team-a"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("init workspace team-a error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(config.WorkspacePackagesDir(home, "team-a")); err != nil {
		t.Fatalf("stat workspace packages dir: %v", err)
	}
}

// TestDisableFromSubdirectoryMatchesStoredRef covers #162: `lineage enable
// ./local-pack` from a subdirectory stores the ref re-expressed against the
// project root, so `lineage disable ./local-pack` from that same directory
// has to normalize its argument the same way before matching. Comparing the
// literal argument made disable reject the exact ref enable had just
// accepted.
func TestDisableFromSubdirectoryMatchesStoredRef(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	sub := filepath.Join(project, "packages", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	rootConfigPath := config.ProjectConfigPath(project)
	if err := config.SaveProjectConfig(rootConfigPath, config.DefaultProjectConfig()); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(sub, "local-pack")
	if err := packages.InitPackage(pkgDir, "local-pack"); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./local-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := Execute(nil, []string{"disable", "./local-pack", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("disable error = %v stderr=%s; disable must normalize the ref the same way enable did", err, stderr.String())
	}

	rootCfg, err := config.LoadProjectConfig(rootConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootCfg.EnabledPackages) != 0 {
		t.Fatalf("root config EnabledPackages = %#v, want empty after disable", rootCfg.EnabledPackages)
	}
}
