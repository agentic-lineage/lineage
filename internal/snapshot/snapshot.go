// Package snapshot is a content-addressed, immutable object store for package snapshots.
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/agentic-lineage/lineage/internal/packages"
)

type ObjectID string

var objectIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

const CurrentManifestSchema = 1

type Manifest struct {
	Schema int `json:"schema"`
	Name string `json:"name"`
	Version string `json:"version"`
	Files []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path string `json:"path"`
	Object ObjectID `json:"object"`
}

func hashID(data []byte) ObjectID {
	sum := sha256.Sum256(data)
	return ObjectID("sha256:" + hex.EncodeToString(sum[:]))
}

func blobPath(root string, id ObjectID) (string, error) {
	if !objectIDPattern.MatchString(string(id)) {
		return "", fmt.Errorf("invalid object id %q", id)
	}
	hexPart := string(id)[len("sha256:"):]
	return filepath.Join(root, hexPart[:2], hexPart[2:]), nil
}

func putBlob(root string, data []byte) (ObjectID, error) {
	id := hashID(data)
	path, err := blobPath(root, id)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		if _, err := getBlob(root, id); err != nil {
			return "", fmt.Errorf("verify existing object %s: %w", id, err)
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lineage-object-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Link(tmpName, path); err != nil {
		if _, verifyErr := getBlob(root, id); verifyErr == nil {
			return id, nil
		} else if os.IsExist(err) {
			return "", fmt.Errorf("verify existing object %s: %w", id, verifyErr)
		} else {
			return "", fmt.Errorf("commit object %s: %w", id, err)
		}
	}
	return id, nil
}

func getBlob(root string, id ObjectID) ([]byte, error) {
	path, err := blobPath(root, id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if got := hashID(data); got != id {
		return nil, fmt.Errorf("object %s is corrupt: content hashes to %s", id, got)
	}
	return data, nil
}

func WriteObject(home string, data []byte) (ObjectID, error) {
	return putBlob(config.ObjectsDir(home), data)
}

func ReadObject(home string, id ObjectID) ([]byte, error) {
	return getBlob(config.ObjectsDir(home), id)
}

func VerifyObject(home string, id ObjectID) error {
	_, err := ReadObject(home, id)
	return err
}

func HasObject(home string, id ObjectID) (bool, error) {
	_, err := ReadObject(home, id)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func manifestBytes(m Manifest) ([]byte, error) { return json.MarshalIndent(m, "", "  ") }

func Create(home, dir string) (Manifest, ObjectID, error) {
	manifest, err := packages.LoadManifest(dir)
	if err != nil {
		return Manifest{}, "", err
	}
	relPaths, err := packages.ContentFiles(dir)
	if err != nil {
		return Manifest{}, "", err
	}
	relPaths = append(relPaths, packages.ManifestFileName)
	sort.Strings(relPaths)
	files := make([]ManifestFile, 0, len(relPaths))
	for _, rel := range relPaths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return Manifest{}, "", fmt.Errorf("read %s for snapshot: %w", rel, err)
		}
		id, err := WriteObject(home, data)
		if err != nil {
			return Manifest{}, "", fmt.Errorf("store object for %s: %w", rel, err)
		}
		files = append(files, ManifestFile{Path: rel, Object: id})
	}
	snapshotManifest := Manifest{Schema: CurrentManifestSchema, Name: manifest.Name, Version: manifest.Version, Files: files}
	data, err := manifestBytes(snapshotManifest)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("encode snapshot manifest: %w", err)
	}
	id, err := putBlob(config.SnapshotsDir(home), data)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("store snapshot manifest: %w", err)
	}
	return snapshotManifest, id, nil
}

func LoadManifest(home string, id ObjectID) (Manifest, error) {
	data, err := getBlob(config.SnapshotsDir(home), id)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse snapshot manifest %s: %w", id, err)
	}
	return m, nil
}

func Materialize(home string, m Manifest, destDir string) error {
	contents := make(map[string][]byte, len(m.Files))
	for _, f := range m.Files {
		data, err := ReadObject(home, f.Object)
		if err != nil {
			return fmt.Errorf("verify object for %s: %w", f.Path, err)
		}
		contents[f.Path] = data
	}
	for _, f := range m.Files {
		dest, err := packages.SafeJoin(destDir, f.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, contents[f.Path], 0o644); err != nil {
			return err
		}
	}
	return nil
}
