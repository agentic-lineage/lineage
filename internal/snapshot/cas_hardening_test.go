package snapshot

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agentic-lineage/lineage/internal/config"
)

func TestReadObjectRejectsMalformedObjectID(t *testing.T) {
	ids := []ObjectID{
		"sha256:abc",
		"sha256:GGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG",
		"sha512:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/../escape",
	}
	for _, id := range ids {
		if _, err := ReadObject(t.TempDir(), id); err == nil {
			t.Fatalf("ReadObject(%q) error = nil, want invalid ID error", id)
		}
	}
}

func TestWriteObjectRefusesToReplaceCorruptExistingObject(t *testing.T) {
	home := t.TempDir()
	data := []byte("payload")
	id := hashID(data)
	path, err := blobPath(config.ObjectsDir(home), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteObject(home, data); err == nil {
		t.Fatal("WriteObject() error = nil, want corrupt existing object error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "tampered" {
		t.Fatalf("WriteObject() replaced corrupt object with %q", got)
	}
}

func TestConcurrentWriteObjectConvergesOnVerifiedObject(t *testing.T) {
	home := t.TempDir()
	data := []byte("concurrent payload")
	const writers = 16
	ids := make(chan ObjectID, writers)
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := WriteObject(home, data)
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Errorf("WriteObject() error = %v", err)
	}
	want := hashID(data)
	for id := range ids {
		if id != want {
			t.Errorf("WriteObject() id = %s, want %s", id, want)
		}
	}
	if err := VerifyObject(home, want); err != nil {
		t.Fatalf("VerifyObject() error = %v", err)
	}
}

func TestHasObjectDistinguishesMissingAndCorrupt(t *testing.T) {
	home := t.TempDir()
	missing := ObjectID("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	got, err := HasObject(home, missing)
	if err != nil || got {
		t.Fatalf("HasObject(missing) = %v, %v; want false, nil", got, err)
	}

	id, err := WriteObject(home, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := blobPath(config.ObjectsDir(home), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := HasObject(home, id); err == nil || got {
		t.Fatalf("HasObject(corrupt) = %v, %v; want false, non-nil", got, err)
	}
}
