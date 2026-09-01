package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

func TestPackagePublishUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// A single-arg "package publish" (no path) is caught by runPackage's own
	// top-level arg-count gate before it ever reaches the publish-specific
	// usage check - same as every other package subcommand. Passing too many
	// args instead is what actually reaches runPackagePublish's own usage
	// message.
	err := Execute(nil, []string{"package", "publish", "a", "b"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("publish with too many args: error = nil, want usage error")
	}
	if !strings.Contains(stderr.String(), "usage: lineage package publish <path>") {
		t.Fatalf("stderr = %q, want publish usage message", stderr.String())
	}
}

func TestPackagePublishRequiresToken(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "needs-token")
	if err := packages.InitPackage(pkgDir, "needs-token"); err != nil {
		t.Fatal(err)
	}

	// Isolate LINEAGE_HOME to an empty temp dir: resolveGitHubToken falls
	// back to whatever `lineage login` stored on disk, and this test must
	// not accidentally pick up a real token file from the machine it runs
	// on.
	t.Setenv(config.HomeEnv, filepath.Join(root, "home"))
	t.Setenv("LINEAGE_PUBLISH_TOKEN", "")
	t.Setenv("LINEAGE_REGISTRY_URL", "http://unused.invalid")

	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"package", "publish", pkgDir, "--yes"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("publish without a token: error = nil, want error")
	}
	if !strings.Contains(stderr.String(), "not logged in") {
		t.Fatalf("stderr = %q, want a message pointing at `lineage login`", stderr.String())
	}
}

func TestPackagePublishSucceeds(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "publish-me")
	if err := packages.InitPackage(pkgDir, "publish-me"); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"alreadyPublished": false})
	}))
	defer srv.Close()

	t.Setenv(config.HomeEnv, filepath.Join(root, "home"))
	t.Setenv("LINEAGE_PUBLISH_TOKEN", "test-token")
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "publish", pkgDir, "--yes"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("publish error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "published publish-me@0.1.0") {
		t.Fatalf("stdout = %q, want confirmation naming the published package", stdout.String())
	}
}

func TestPackagePublishRefusesPortabilityBlockers(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "blocked-publish")
	if err := packages.InitPackage(pkgDir, "blocked-publish"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, ".env"), []byte("API_KEY=whatever"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.HomeEnv, filepath.Join(root, "home"))
	t.Setenv("LINEAGE_PUBLISH_TOKEN", "test-token")
	t.Setenv("LINEAGE_REGISTRY_URL", "http://unused.invalid")

	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"package", "publish", pkgDir, "--yes"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("publish error = nil, want portability blocker error")
	}
	if !strings.Contains(stderr.String(), "unresolved portability blockers") {
		t.Fatalf("stderr = %q, want portability blocker message", stderr.String())
	}
}

// TestPackagePublishInteractiveYesProceeds covers the core gate from #163:
// without --yes, an affirmative answer must still reach the registry.
func TestPackagePublishInteractiveYesProceeds(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "confirm-yes")
	if err := packages.InitPackage(pkgDir, "confirm-yes"); err != nil {
		t.Fatal(err)
	}

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"alreadyPublished": false})
	}))
	defer srv.Close()

	t.Setenv(config.HomeEnv, filepath.Join(root, "home"))
	t.Setenv("LINEAGE_PUBLISH_TOKEN", "test-token")
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\n")
	if err := Execute(nil, []string{"package", "publish", pkgDir}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("publish error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Publish package at") {
		t.Fatalf("stdout = %q, want a publish confirmation prompt", stdout.String())
	}
	if !strings.Contains(stdout.String(), "published confirm-yes@0.1.0") {
		t.Fatalf("stdout = %q, want confirmation naming the published package", stdout.String())
	}
	if hits == 0 {
		t.Fatal("expected the registry to be called after interactive yes")
	}
}

// TestPackagePublishDeclinedAbortsWithoutRegistry covers declined and
// no-input paths: both must print "aborted" and never hit the registry.
func TestPackagePublishDeclinedAbortsWithoutRegistry(t *testing.T) {
	cases := []struct {
		name  string
		stdin io.Reader // nil means no stdin reader at all
	}{
		{name: "declined", stdin: strings.NewReader("n\n")},
		{name: "no-input", stdin: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			pkgDir := filepath.Join(root, "confirm-no")
			if err := packages.InitPackage(pkgDir, "confirm-no"); err != nil {
				t.Fatal(err)
			}

			hits := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"alreadyPublished": false})
			}))
			defer srv.Close()

			t.Setenv(config.HomeEnv, filepath.Join(root, "home"))
			t.Setenv("LINEAGE_PUBLISH_TOKEN", "test-token")
			t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)

			var stdout, stderr bytes.Buffer
			if err := Execute(nil, []string{"package", "publish", pkgDir}, tc.stdin, &stdout, &stderr); err != nil {
				t.Fatalf("publish error = %v stderr=%s", err, stderr.String())
			}
			if !strings.Contains(stdout.String(), "aborted") {
				t.Fatalf("stdout = %q, want abort notice", stdout.String())
			}
			if hits != 0 {
				t.Fatalf("registry hits = %d, want 0 when publish is declined", hits)
			}
		})
	}
}

func TestPackagePullUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Same top-level gate as TestPackagePublishUsage: a malformed --as flag
	// (missing its value) is what actually reaches runPackagePull's own
	// usage message.
	err := Execute(nil, []string{"package", "pull", "some-ref", "--as"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("pull with a dangling --as flag: error = nil, want usage error")
	}
	if !strings.Contains(stderr.String(), "usage: lineage package pull <package-ref>") {
		t.Fatalf("stderr = %q, want pull usage message", stderr.String())
	}
}

func TestPackagePullImportsIntoUserPackages(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	srcDir := filepath.Join(tmp, "pull-me")
	if err := packages.InitPackage(srcDir, "pull-me"); err != nil {
		t.Fatal(err)
	}
	report, err := packages.Validate(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := packages.Export(srcDir, &archive); err != nil {
		t.Fatal(err)
	}
	archiveBytes := archive.Bytes()

	ref := "pull-me@0.1.0"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/packages/"+ref, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "pull-me", "version": "0.1.0", "digest": report.Digest,
			"downloadPath": "/api/packages/" + ref + "/download",
		})
	})
	mux.HandleFunc("/api/packages/"+ref+"/download", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldHome := os.Getenv(config.HomeEnv)
	t.Setenv(config.HomeEnv, home)
	defer t.Setenv(config.HomeEnv, oldHome)
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "pull", ref}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("pull error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pulled package pull-me") {
		t.Fatalf("stdout = %q, want confirmation naming the pulled package", stdout.String())
	}

	importedDir := filepath.Join(config.UserPackagesDir(home), "pull-me")
	if _, err := os.Stat(filepath.Join(importedDir, packages.ManifestFileName)); err != nil {
		t.Fatalf("expected pulled manifest: %v", err)
	}
}
