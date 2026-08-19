package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lineage-dev/lineage/internal/config"
	"github.com/lineage-dev/lineage/internal/packages"
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

	t.Setenv("LINEAGE_PUBLISH_TOKEN", "")
	t.Setenv("LINEAGE_REGISTRY_URL", "http://unused.invalid")

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "publish", pkgDir}, nil, &stdout, &stderr); err == nil {
		t.Fatal("publish without a token: error = nil, want error")
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

	t.Setenv("LINEAGE_PUBLISH_TOKEN", "test-token")
	t.Setenv("LINEAGE_REGISTRY_URL", srv.URL)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"package", "publish", pkgDir}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("publish error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "published publish-me@0.1.0") {
		t.Fatalf("stdout = %q, want confirmation naming the published package", stdout.String())
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
