package distribution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agentic-lineage/lineage/internal/packages"
	"github.com/agentic-lineage/lineage/internal/snapshot"
)

type contentFixture struct {
	manifest snapshot.ContentManifest
	objects  map[string][]byte
}

func makeContentFixture(t *testing.T, name, version, hello string) contentFixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := packages.InitPackage(dir, name); err != nil {
		t.Fatal(err)
	}
	manifest, err := packages.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Version = version
	if err := packages.SaveManifest(dir, manifest); err != nil {
		t.Fatal(err)
	}
	for rel, data := range map[string]string{
		"skills/hello/SKILL.md": hello,
		"skills/world/SKILL.md": "# World\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := snapshot.BuildContentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	objects := make(map[string][]byte, len(m.Assets))
	for _, asset := range m.Assets {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(asset.Path)))
		if err != nil {
			t.Fatal(err)
		}
		objects[string(asset.Object)] = data
	}
	return contentFixture{manifest: m, objects: objects}
}

type contentServer struct {
	server *httptest.Server
	mu     sync.Mutex
	hits   map[string]int
	fail   map[string]int
	data   map[string][]byte
}

func newContentServer(t *testing.T, fixtures map[string]contentFixture) *contentServer {
	t.Helper()
	s := &contentServer{hits: make(map[string]int), fail: make(map[string]int), data: make(map[string][]byte)}
	for _, fixture := range fixtures {
		for id, data := range fixture.objects {
			s.data[id] = data
		}
	}
	mux := http.NewServeMux()
	for ref, fixture := range fixtures {
		ref, fixture := ref, fixture
		mux.HandleFunc("/api/packages/"+ref, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": fixture.manifest.Name, "version": fixture.manifest.Version, "digest": fixture.manifest.PackageDigest,
				"contentManifest": fixture.manifest, "objectPathTemplate": "/api/objects/{digest}",
			})
		})
	}
	mux.HandleFunc("/api/objects/", func(w http.ResponseWriter, r *http.Request) {
		id, err := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), "/api/objects/"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.hits[id]++
		if s.fail[id] > 0 {
			s.fail[id]--
			s.mu.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "temporary failure")
			return
		}
		data, ok := s.data[id]
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	})
	s.server = httptest.NewServer(mux)
	return s
}

func (s *contentServer) count(id snapshot.ObjectID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[string(id)]
}

func TestPullTransfersOnlyMissingObjectsAndWorksOffline(t *testing.T) {
	fixture := makeContentFixture(t, "fresh-pack", "0.1.0", "# Hello\n")
	srv := newContentServer(t, map[string]contentFixture{"fresh-pack@0.1.0": fixture})
	home := t.TempDir()
	destParent := t.TempDir()
	name, err := Pull("fresh-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, destParent, "")
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if name != fixture.manifest.Name {
		t.Fatalf("Pull() name = %q, want %q", name, fixture.manifest.Name)
	}
	for id := range fixture.objects {
		if got := srv.count(snapshot.ObjectID(id)); got != 1 {
			t.Errorf("object %s fetched %d times, want once", id, got)
		}
	}
	srv.server.Close()
	if err := snapshot.MaterializeRelease(home, fixture.manifest.Name, fixture.manifest.Version, filepath.Join(t.TempDir(), "offline")); err != nil {
		t.Fatalf("MaterializeRelease() after registry shutdown error = %v", err)
	}
}

func TestPullReusesSharedObjectsAcrossPackages(t *testing.T) {
	first := makeContentFixture(t, "first-pack", "0.1.0", "# Shared\n")
	second := makeContentFixture(t, "second-pack", "0.1.0", "# Shared\n")
	srv := newContentServer(t, map[string]contentFixture{"first-pack@0.1.0": first, "second-pack@0.1.0": second})
	home, dest := t.TempDir(), t.TempDir()
	if _, err := Pull("first-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, dest, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull("second-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, dest, ""); err != nil {
		t.Fatal(err)
	}
	var shared snapshot.ObjectID
	for _, asset := range first.manifest.Assets {
		if asset.Path == "skills/hello/SKILL.md" {
			shared = asset.Object
		}
	}
	if got := srv.count(shared); got != 1 {
		t.Fatalf("shared object fetched %d times, want once", got)
	}
}

func TestPullVersionUpdateFetchesOnlyChangedObject(t *testing.T) {
	first := makeContentFixture(t, "update-pack", "0.1.0", "# Before\n")
	second := makeContentFixture(t, "update-pack", "0.2.0", "# After\n")
	srv := newContentServer(t, map[string]contentFixture{"update-pack@0.1.0": first, "update-pack@0.2.0": second})
	home, dest := t.TempDir(), t.TempDir()
	if _, err := Pull("update-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, dest, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull("update-pack@0.2.0", packages.RegistryConfig{URL: srv.server.URL}, home, dest, "update-pack-v2"); err != nil {
		t.Fatal(err)
	}
	for _, asset := range second.manifest.Assets {
		if asset.Path == "skills/world/SKILL.md" && srv.count(asset.Object) != 1 {
			t.Fatalf("unchanged object fetched %d times, want once", srv.count(asset.Object))
		}
	}
}

func TestPullRetryReusesObjectsAfterPartialFailure(t *testing.T) {
	fixture := makeContentFixture(t, "retry-pack", "0.1.0", "# Hello\n")
	srv := newContentServer(t, map[string]contentFixture{"retry-pack@0.1.0": fixture})
	var failed snapshot.ObjectID
	for _, asset := range fixture.manifest.Assets {
		if asset.Path == "skills/world/SKILL.md" {
			failed = asset.Object
		}
	}
	srv.fail[string(failed)] = 1
	home, dest := t.TempDir(), t.TempDir()
	if _, err := Pull("retry-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, dest, ""); err == nil {
		t.Fatal("Pull() error = nil on partial failure, want error")
	}
	if _, err := snapshot.LoadRelease(home, fixture.manifest.Name, fixture.manifest.Version); !os.IsNotExist(err) {
		t.Fatalf("LoadRelease() error = %v after failed transfer, want IsNotExist", err)
	}
	if _, err := Pull("retry-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, dest, ""); err != nil {
		t.Fatalf("Pull() retry error = %v", err)
	}
	for _, asset := range fixture.manifest.Assets {
		if asset.Object != failed && srv.count(asset.Object) != 1 {
			t.Errorf("verified object %s fetched %d times after retry, want once", asset.Object, srv.count(asset.Object))
		}
	}
}

func TestPullRejectsMismatchedObjectWithoutStoringIt(t *testing.T) {
	fixture := makeContentFixture(t, "mismatch-pack", "0.1.0", "# Hello\n")
	srv := newContentServer(t, map[string]contentFixture{"mismatch-pack@0.1.0": fixture})
	var target snapshot.ObjectID
	for _, asset := range fixture.manifest.Assets {
		if asset.Path == "skills/hello/SKILL.md" {
			target = asset.Object
		}
	}
	srv.data[string(target)] = []byte("tampered")
	home := t.TempDir()
	if _, err := Pull("mismatch-pack@0.1.0", packages.RegistryConfig{URL: srv.server.URL}, home, t.TempDir(), ""); err == nil {
		t.Fatal("Pull() error = nil for a mismatched object, want error")
	}
	status, err := snapshot.ObjectAvailability(home, target)
	if err != nil || status != snapshot.ObjectMissing {
		t.Fatalf("ObjectAvailability() = %v, %v; want ObjectMissing, nil", status, err)
	}
}

func TestPullFallsBackToArchiveAndRegistersRelease(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "archive-pack")
	if err := packages.InitPackage(dir, "archive-pack"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "review", "SKILL.md"), []byte("# Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := packages.Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := packages.Export(dir, &archive); err != nil {
		t.Fatal(err)
	}
	ref := "archive-pack@0.1.0"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/packages/"+ref, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "archive-pack", "version": "0.1.0", "digest": report.Digest, "downloadPath": "/download"})
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive.Bytes()) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	home := t.TempDir()
	if _, err := Pull(ref, packages.RegistryConfig{URL: srv.URL}, home, t.TempDir(), ""); err != nil {
		t.Fatalf("Pull() archive fallback error = %v", err)
	}
	if _, err := snapshot.LoadRelease(home, "archive-pack", "0.1.0"); err != nil {
		t.Fatalf("LoadRelease() after archive fallback error = %v", err)
	}
}
