package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/graph"
	"github.com/agentic-lineage/lineage/internal/snapshot"
)

func TestEnableRecordsGraphEntry(t *testing.T) {
	project, home := setUpEnabledProject(t)

	records, err := graph.Load(project)
	if err != nil {
		t.Fatalf("graph.Load() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("graph.Load() = %d records after one enable, want 1", len(records))
	}
	rec := records[0]
	if rec.Event != "enable" {
		t.Errorf("Event = %q, want %q", rec.Event, "enable")
	}
	if rec.Parent.Name != "agent-pack" || rec.Parent.Version != "0.1.0" {
		t.Errorf("Parent = %+v, want name=agent-pack version=0.1.0", rec.Parent)
	}
	if rec.Parent.Digest == "" {
		t.Error("Parent.Digest is empty, want a computed digest")
	}
	if rec.ID == "" {
		t.Error("ID is empty")
	}
	if rec.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	if rec.Parent.SnapshotID == "" {
		t.Fatal("Parent.SnapshotID is empty, want a snapshot created by enableRef")
	}
	if _, err := snapshot.LoadManifest(home, snapshot.ObjectID(rec.Parent.SnapshotID)); err != nil {
		t.Fatalf("snapshot.LoadManifest(%q) error = %v, want the snapshot enableRef created to be loadable", rec.Parent.SnapshotID, err)
	}
}

// TestEnableFailingGraphAppendDoesNotPersistConfig covers a regression
// where enableRef saved .lineage/config.yaml (marking the package enabled)
// before computing the digest, taking the snapshot, and appending to the
// graph - any of which can still fail. A corrupt .lineage/graph.json (the
// kind a prior crash mid-write could leave behind) made graph.Append fail
// after config had already been saved: `enable` reported a non-zero exit
// while the package was already durably marked enabled, with every
// subsequent enable in the project failing the same opaque way.
func TestEnableFailingGraphAppendDoesNotPersistConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	pkgDir := filepath.Join(project, "agent-pack")

	if err := os.MkdirAll(filepath.Join(pkgDir, "skills", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "lineage.yaml"), []byte("name: agent-pack\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "skills", "hello", "SKILL.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A corrupt graph.json - the kind a crash mid-write could leave behind
	// - makes graph.Append fail at the Load step, before it ever writes
	// anything.
	if err := os.MkdirAll(filepath.Join(project, ".lineage"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".lineage", "graph.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	t.Cleanup(func() { os.Setenv(config.HomeEnv, oldHome) })
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err == nil {
		t.Fatal("enable error = nil, want error from the corrupt graph file")
	}

	found, err := config.FindProjectConfig(project)
	if err != nil {
		// No config was ever written - the ideal outcome for a first
		// enable that failed before completing.
		return
	}
	if contains(found.Config.EnabledPackages, "./agent-pack") {
		t.Errorf("enabled packages = %v, want agent-pack NOT enabled after a failed enable", found.Config.EnabledPackages)
	}
}

func TestEnableTwiceAppendsSecondRecord(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("second enable error = %v stderr=%s", err, stderr.String())
	}

	records, err := graph.Load(project)
	if err != nil {
		t.Fatalf("graph.Load() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("graph.Load() = %d records after enabling twice, want 2 (append-only log)", len(records))
	}
}

func TestEnableCreatesGitignoreEntryForNewProject(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	data, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(data), ".lineage/") {
		t.Fatalf(".gitignore = %q, want it to contain .lineage/", data)
	}
}

func TestEnableTwiceDoesNotDuplicateGitignoreEntry(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("second enable error = %v stderr=%s", err, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if strings.Count(string(data), ".lineage/") != 1 {
		t.Fatalf(".gitignore = %q, want exactly one .lineage/ entry", data)
	}
}

func TestEnableDoesNotOverwriteReceiversRemovedGitignoreEntry(t *testing.T) {
	project, _ := setUpEnabledProject(t)

	gitignorePath := filepath.Join(project, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"enable", "./agent-pack"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("second enable error = %v stderr=%s", err, stderr.String())
	}

	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "node_modules/\n" {
		t.Fatalf(".gitignore = %q, want unchanged after a receiver removed the .lineage/ entry post-first-enable", data)
	}
}

func TestGraphListHumanOutput(t *testing.T) {
	setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"graph", "list"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("graph list error = %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "agent-pack@0.1.0") {
		t.Fatalf("graph list output = %s, want it to mention agent-pack@0.1.0", out)
	}
	if !strings.Contains(out, "enable") {
		t.Fatalf("graph list output = %s, want it to mention the enable event", out)
	}
}

func TestGraphListYAMLOutput(t *testing.T) {
	setUpEnabledProject(t)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"graph", "list", "--yaml"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("graph list --yaml error = %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "name: agent-pack") {
		t.Fatalf("graph list --yaml output = %s, want it to mention name: agent-pack", out)
	}
	if !strings.Contains(out, "event: enable") {
		t.Fatalf("graph list --yaml output = %s, want it to mention event: enable", out)
	}
	if !strings.Contains(out, "snapshot_id: sha256:") {
		t.Fatalf("graph list --yaml output = %s, want it to mention a snapshot_id", out)
	}
}

func TestGraphListWithNoRecordsSaysSo(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	project := filepath.Join(tmp, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProjectConfig(config.ProjectConfigPath(project), config.DefaultProjectConfig()); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, home)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"graph", "list"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("graph list error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no lineage graph records") {
		t.Fatalf("graph list output = %q, want a clear empty message", stdout.String())
	}
}
