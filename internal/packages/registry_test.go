package packages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestPublishIncludesEntrypointsAndCapabilities covers #90: provider
// compatibility and capabilities are already public, declarative
// information once a package validates - Publish must send them to the
// registry so a receiver can see them before ever pulling the package.
func TestPublishIncludesEntrypointsAndCapabilities(t *testing.T) {
	root := filepath.Join(t.TempDir(), "capable-pack")
	if err := InitPackage(root, "capable-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review")
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Entrypoints = Entrypoints{Claude: "skills/review/SKILL.md"}
	manifest.Capabilities = Capabilities{
		Filesystem: FilesystemCapabilities{Read: []string{"./data"}},
		Network:    []string{"api.example.com"},
	}
	if err := SaveManifest(root, manifest); err != nil {
		t.Fatal(err)
	}

	var gotEntrypoints, gotCapabilities string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEntrypoints = r.URL.Query().Get("entrypoints")
		gotCapabilities = r.URL.Query().Get("capabilities")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"alreadyPublished": false})
	}))
	defer srv.Close()

	if _, err := Publish(root, RegistryConfig{URL: srv.URL, Token: "test-token"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	var entrypoints Entrypoints
	if err := json.Unmarshal([]byte(gotEntrypoints), &entrypoints); err != nil {
		t.Fatalf("entrypoints query param = %q, not valid JSON: %v", gotEntrypoints, err)
	}
	if entrypoints.Claude != "skills/review/SKILL.md" {
		t.Errorf("entrypoints.Claude = %q, want skills/review/SKILL.md", entrypoints.Claude)
	}

	var capabilities Capabilities
	if err := json.Unmarshal([]byte(gotCapabilities), &capabilities); err != nil {
		t.Fatalf("capabilities query param = %q, not valid JSON: %v", gotCapabilities, err)
	}
	if len(capabilities.Filesystem.Read) != 1 || capabilities.Filesystem.Read[0] != "./data" {
		t.Errorf("capabilities.Filesystem.Read = %v, want [./data]", capabilities.Filesystem.Read)
	}
	if len(capabilities.Network) != 1 || capabilities.Network[0] != "api.example.com" {
		t.Errorf("capabilities.Network = %v, want [api.example.com]", capabilities.Network)
	}
}

func TestPublishUploadsExportedArchive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "publish-pack")
	if err := InitPackage(root, "publish-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "skills", "review", "SKILL.md"), "# Review")

	report, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}

	var gotArchive []byte
	var gotAuth, gotName, gotVersion, gotDigest string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/publish" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotName = r.URL.Query().Get("name")
		gotVersion = r.URL.Query().Get("version")
		gotDigest = r.URL.Query().Get("digest")
		gotArchive, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": gotName, "version": gotVersion, "digest": gotDigest, "alreadyPublished": false,
		})
	}))
	defer srv.Close()

	result, err := Publish(root, RegistryConfig{URL: srv.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want Bearer test-token", gotAuth)
	}
	if gotName != "publish-pack" || gotVersion != report.Manifest.Version || gotDigest != report.Digest {
		t.Errorf("query params = name=%q version=%q digest=%q, want name=publish-pack version=%q digest=%q", gotName, gotVersion, gotDigest, report.Manifest.Version, report.Digest)
	}
	if len(gotArchive) == 0 {
		t.Error("Publish() sent an empty archive body")
	}
	if result.AlreadyPublished {
		t.Error("Publish() result.AlreadyPublished = true, want false")
	}
	if result.Digest != report.Digest {
		t.Errorf("Publish() result.Digest = %q, want %q", result.Digest, report.Digest)
	}
}

func TestPublishRefusesWhenValidationFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken-pack")
	if err := InitPackage(root, "broken-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".env"), "API_KEY=whatever")

	if _, err := Publish(root, RegistryConfig{URL: "http://unused.invalid", Token: "t"}); err == nil {
		t.Fatal("Publish() error = nil, want error for a package that fails validation")
	}
}

func TestPublishRequiresToken(t *testing.T) {
	root := filepath.Join(t.TempDir(), "token-pack")
	if err := InitPackage(root, "token-pack"); err != nil {
		t.Fatal(err)
	}

	if _, err := Publish(root, RegistryConfig{URL: "http://unused.invalid"}); err == nil {
		t.Fatal("Publish() error = nil, want error when no token is configured")
	}
}

func TestPublishSurfacesConflictOnDigestMismatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "conflict-pack")
	if err := InitPackage(root, "conflict-pack"); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"already published with a different digest"}`))
	}))
	defer srv.Close()

	_, err := Publish(root, RegistryConfig{URL: srv.URL, Token: "t"})
	if err == nil {
		t.Fatal("Publish() error = nil, want error on 409 from the registry")
	}
}

// pullTestServer serves the same contract as landing/api: metadata at
// /api/packages/:ref, archive bytes at /api/packages/:ref/download.
func pullTestServer(t *testing.T, ref string, meta map[string]any, archive []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/packages/"+ref, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(meta)
	})
	mux.HandleFunc("/api/packages/"+ref+"/download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	})
	return httptest.NewServer(mux)
}

func TestPullImportsAndVerifiesDigest(t *testing.T) {
	src := filepath.Join(t.TempDir(), "pull-pack")
	if err := InitPackage(src, "pull-pack"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(src, "skills", "review", "SKILL.md"), "# Review")

	report, err := Validate(src)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := Export(src, &archive); err != nil {
		t.Fatal(err)
	}

	ref := "pull-pack@0.1.0"
	srv := pullTestServer(t, ref, map[string]any{
		"name": "pull-pack", "version": "0.1.0", "digest": report.Digest,
		"downloadPath": "/api/packages/" + ref + "/download",
	}, archive.Bytes())
	defer srv.Close()

	destParent := t.TempDir()
	name, err := Pull(ref, RegistryConfig{URL: srv.URL}, destParent, "")
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if name != "pull-pack" {
		t.Errorf("Pull() name = %q, want pull-pack", name)
	}
	if _, err := os.Stat(filepath.Join(destParent, name, ManifestFileName)); err != nil {
		t.Errorf("pulled package manifest missing: %v", err)
	}
}

func TestPullFailsClosedOnDigestMismatch(t *testing.T) {
	src := filepath.Join(t.TempDir(), "mismatch-pack")
	if err := InitPackage(src, "mismatch-pack"); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := Export(src, &archive); err != nil {
		t.Fatal(err)
	}

	ref := "mismatch-pack@0.1.0"
	srv := pullTestServer(t, ref, map[string]any{
		"name": "mismatch-pack", "version": "0.1.0",
		"digest":       "sha256:0000000000000000000000000000000000000000000000000000000000000",
		"downloadPath": "/api/packages/" + ref + "/download",
	}, archive.Bytes())
	defer srv.Close()

	destParent := t.TempDir()
	if _, err := Pull(ref, RegistryConfig{URL: srv.URL}, destParent, ""); err == nil {
		t.Fatal("Pull() error = nil, want error on digest mismatch")
	}
	if _, err := os.Stat(filepath.Join(destParent, "mismatch-pack")); !os.IsNotExist(err) {
		t.Errorf("Pull() left a package directory behind after a digest mismatch: err = %v", err)
	}
}

func TestPullFailsClosedOnMissingDigest(t *testing.T) {
	src := filepath.Join(t.TempDir(), "no-digest-pack")
	if err := InitPackage(src, "no-digest-pack"); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := Export(src, &archive); err != nil {
		t.Fatal(err)
	}

	ref := "no-digest-pack@0.1.0"
	srv := pullTestServer(t, ref, map[string]any{
		"name": "no-digest-pack", "version": "0.1.0",
		"downloadPath": "/api/packages/" + ref + "/download",
	}, archive.Bytes())
	defer srv.Close()

	destParent := t.TempDir()
	if _, err := Pull(ref, RegistryConfig{URL: srv.URL}, destParent, ""); err == nil {
		t.Fatal("Pull() error = nil, want error when the registry omits the digest")
	}
	if _, err := os.Stat(filepath.Join(destParent, "no-digest-pack")); !os.IsNotExist(err) {
		t.Errorf("Pull() left a package directory behind after a missing-digest response: err = %v", err)
	}
}

func TestPullReportsMissingRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not found"}`)
	}))
	defer srv.Close()

	if _, err := Pull("nope@1.0.0", RegistryConfig{URL: srv.URL}, t.TempDir(), ""); err == nil {
		t.Fatal("Pull() error = nil, want error for an unknown ref")
	}
}

// TestRegistryClientHasTimeout covers #164: the metadata fetch and archive
// download in Pull used http.DefaultClient, which has no timeout, so a hung
// registry stalled `lineage package pull` indefinitely.
func TestRegistryClientHasTimeout(t *testing.T) {
	if got := registryClient().Timeout; got != registryRequestTimeout {
		t.Fatalf("registryClient().Timeout = %v, want %v", got, registryRequestTimeout)
	}
}
