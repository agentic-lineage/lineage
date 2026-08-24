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

// initPackageWithSetup scaffolds a package under project that declares one
// tracker file and one directory as setup requirements (#72).
func initPackageWithSetup(t *testing.T, dir, name string) {
	t.Helper()
	if err := packages.InitPackage(dir, name); err != nil {
		t.Fatal(err)
	}
	manifest, err := packages.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Setup = packages.Setup{
		Files:       []packages.SetupFile{{Path: "tasks.csv", Description: "tracks work items", Template: "title,owner,status\n"}},
		Directories: []packages.SetupDirectory{{Path: "notes", Description: "workflow output"}},
	}
	if err := packages.SaveManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestEnableSetupPromptsAndCreatesOnApproval(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "tracker-pack")
	initPackageWithSetup(t, pkgDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\n")
	if err := Execute(nil, []string{"enable", "./tracker-pack"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"tracker-pack wants to set up", "create file tasks.csv - tracks work items", "create directory notes - workflow output", "setup complete", "enabled package"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want it to contain %q", out, want)
		}
	}

	if got, err := os.ReadFile(filepath.Join(project, "tasks.csv")); err != nil || string(got) != "title,owner,status\n" {
		t.Errorf("tasks.csv = %q, err=%v, want the declared template content", got, err)
	}
	if info, err := os.Stat(filepath.Join(project, "notes")); err != nil || !info.IsDir() {
		t.Error("expected notes/ to be created as a directory")
	}
	found, err := config.FindProjectConfig(project)
	if err != nil {
		t.Fatalf("expected a project config: %v", err)
	}
	if !contains(found.Config.EnabledPackages, "./tracker-pack") {
		t.Errorf("enabled packages = %v, want ./tracker-pack", found.Config.EnabledPackages)
	}
}

func TestEnableSetupDeclinedLeavesWorkspaceUnchanged(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "tracker-pack")
	initPackageWithSetup(t, pkgDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("n\n")
	if err := Execute(nil, []string{"enable", "./tracker-pack"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not enabled - setup was declined") {
		t.Errorf("stdout = %q, want a message confirming setup was declined", stdout.String())
	}

	if _, err := os.Stat(filepath.Join(project, "tasks.csv")); !os.IsNotExist(err) {
		t.Errorf("tasks.csv stat err = %v, want it to not exist after declining", err)
	}
	if _, err := os.Stat(filepath.Join(project, "notes")); !os.IsNotExist(err) {
		t.Errorf("notes/ stat err = %v, want it to not exist after declining", err)
	}
	if _, err := config.FindProjectConfig(project); err == nil {
		t.Error("expected no project config after declining setup - the package must not be enabled either")
	}
}

func TestEnableSetupYesFlagSkipsPrompt(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "tracker-pack")
	initPackageWithSetup(t, pkgDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	// No stdin reader at all - if enable tried to read a confirmation
	// despite --yes, this would fail/hang instead of silently passing.
	if err := Execute(nil, []string{"enable", "./tracker-pack", "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable --yes error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, "tasks.csv")); err != nil {
		t.Errorf("expected tasks.csv to be created under --yes without a prompt: %v", err)
	}
}

func TestEnableSetupAlreadyPresentSkipsPromptEntirely(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "tracker-pack")
	initPackageWithSetup(t, pkgDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	// The receiver already has their own tasks.csv and notes/ - setup has
	// nothing to do, so it must not prompt at all (#72: repeated
	// processing / a project that already has everything is a no-op).
	if err := os.WriteFile(filepath.Join(project, "tasks.csv"), []byte("their own content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	// No stdin reader - a prompt here (with nothing to answer it) would
	// cause a read error, which Execute would surface as a non-nil error.
	if err := Execute(nil, []string{"enable", "./tracker-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("enable error = %v stderr=%s, want no prompt when everything already exists", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "wants to set up") {
		t.Errorf("stdout = %q, want no setup plan printed when nothing needs creating", stdout.String())
	}
	got, err := os.ReadFile(filepath.Join(project, "tasks.csv"))
	if err != nil || string(got) != "their own content\n" {
		t.Errorf("tasks.csv = %q, err=%v, want the receiver's own content left untouched", got, err)
	}
}

// TestEnableSetupReRunIsIdempotent covers #72's "repeated processing" and
// "resumable" requirements directly: enabling the same package a second
// time must not re-prompt (nothing new to create) and must not disturb
// what the first run already created.
func TestEnableSetupReRunIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "tracker-pack")
	initPackageWithSetup(t, pkgDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var first bytes.Buffer
	if err := Execute(nil, []string{"enable", "./tracker-pack", "--yes"}, nil, &first, &first); err != nil {
		t.Fatalf("first enable error = %v output=%s", err, first.String())
	}
	if err := os.WriteFile(filepath.Join(project, "tasks.csv"), []byte("edited by the receiver\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var second bytes.Buffer
	// No stdin - a second, unwanted setup prompt here would fail the test
	// instead of silently passing, since NeedsAction() should be false.
	if err := Execute(nil, []string{"enable", "./tracker-pack"}, nil, &second, &second); err != nil {
		t.Fatalf("second enable error = %v output=%s", err, second.String())
	}
	if strings.Contains(second.String(), "wants to set up") {
		t.Errorf("second enable output = %q, want no setup plan - everything already exists", second.String())
	}

	got, err := os.ReadFile(filepath.Join(project, "tasks.csv"))
	if err != nil || string(got) != "edited by the receiver\n" {
		t.Errorf("tasks.csv = %q, err=%v, want the receiver's edit preserved across the re-run", got, err)
	}
}

func TestAddWithSetupYesAppliesWithoutPrompt(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	srcDir := filepath.Join(tmp, "tracker-pack")
	initPackageWithSetup(t, srcDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	ref := "tracker-pack@0.1.0"
	srv := addTestServer(t, ref, srcDir)
	defer srv.Close()

	t.Setenv(config.HomeEnv, home)
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"add", ref, "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("add --yes error = %v stderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(project, "tasks.csv")); err != nil {
		t.Errorf("expected tasks.csv to be created via add --yes without a prompt: %v", err)
	}
}

// TestAddDecliningSetupDoesNotReportReady covers a regression where `add`
// (no --yes), for a package with setup, answered "yes" to enabling but "no"
// to creating its setup files - enableRef correctly declined and left the
// workspace unchanged, but runAdd only checked enableRef's error (nil, by
// design, since a declined prompt isn't a failure) and unconditionally
// printed "Ready. Run `lineage run ...`" anyway, telling the user a package
// was ready to use when it was never actually enabled.
func TestAddDecliningSetupDoesNotReportReady(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	srcDir := filepath.Join(tmp, "tracker-pack")
	initPackageWithSetup(t, srcDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	ref := "tracker-pack@0.1.0"
	srv := addTestServer(t, ref, srcDir)
	defer srv.Close()

	t.Setenv(config.HomeEnv, home)
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\nn\n") // yes to enabling, no to setup
	if err := Execute(nil, []string{"add", ref}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("add error = %v stderr=%s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "not enabled - setup was declined") {
		t.Errorf("stdout = %q, want it to explain setup was declined", out)
	}
	if strings.Contains(out, "Ready.") {
		t.Errorf("stdout = %q, must not claim readiness for a package that was never enabled", out)
	}
	if _, err := config.FindProjectConfig(project); err == nil {
		t.Error("expected no project config after declining setup - the package must not be marked enabled")
	}
	if _, err := os.Stat(filepath.Join(project, "tasks.csv")); !os.IsNotExist(err) {
		t.Errorf("tasks.csv should not have been created after declining setup: err = %v", err)
	}
}

// TestAddRespectsBothPromptsFromSingleBufferedStdin covers a regression
// where `add` (no --yes) answers two prompts in one run - "enable this
// package?" then, for a package with setup, "create these files?" - and
// each confirm() call used to construct its own bufio.Reader over the same
// stdin. A single Read() delivering both answered lines at once (true for
// strings.Reader, and for real piped/scripted stdin) meant the first
// bufio.Reader buffered and discarded the second line before the second
// confirm() call ever saw it, so a real "y\ny\n" silently behaved as if
// the setup prompt had been declined.
func TestAddRespectsBothPromptsFromSingleBufferedStdin(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	srcDir := filepath.Join(tmp, "tracker-pack")
	initPackageWithSetup(t, srcDir, "tracker-pack")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	ref := "tracker-pack@0.1.0"
	srv := addTestServer(t, ref, srcDir)
	defer srv.Close()

	t.Setenv(config.HomeEnv, home)
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)
	oldWd, _ := os.Getwd()
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\n")
	if err := Execute(nil, []string{"add", ref}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("add error = %v stderr=%s", err, stderr.String())
	}

	found, err := config.FindProjectConfig(project)
	if err != nil {
		t.Fatalf("expected a project config after answering yes to both prompts: %v", err)
	}
	if !contains(found.Config.EnabledPackages, "tracker-pack") {
		t.Errorf("enabled packages = %v, want tracker-pack (the enable prompt's \"y\" must not be lost)", found.Config.EnabledPackages)
	}
	if _, err := os.Stat(filepath.Join(project, "tasks.csv")); err != nil {
		t.Errorf("expected tasks.csv to be created: the setup prompt's \"y\" must not be lost: %v", err)
	}
	if !strings.Contains(stdout.String(), "Ready. Run `lineage run") {
		t.Errorf("stdout = %q, want the success message once setup was actually approved and applied", stdout.String())
	}
}
