// Package distribution resolves registry releases into the local verified
// content store. It keeps registry transport policy separate from package
// validation and CAS storage so neither layer needs to know the other's
// on-disk layout.
package distribution

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-lineage/lineage/internal/packages"
	"github.com/agentic-lineage/lineage/internal/snapshot"
)

const (
	registryRequestTimeout = 60 * time.Second
	maxObjectDownloadSize  = 50 << 20
)

type metadata struct {
	Name               string                    `json:"name"`
	Version            string                    `json:"version"`
	Digest             string                    `json:"digest"`
	ContentManifest    *snapshot.ContentManifest `json:"contentManifest"`
	ObjectPathTemplate string                    `json:"objectPathTemplate"`
}

// Pull fetches a registry release into destParent. Content-manifest-aware
// registries transfer only missing verified objects; legacy registries keep
// the existing archive flow and migrate the verified result into the CAS.
func Pull(ref string, cfg packages.RegistryConfig, home, destParent, asName string) (string, error) {
	meta, err := fetchMetadata(ref, cfg)
	if err != nil {
		return "", err
	}
	if meta.ContentManifest == nil {
		return pullArchive(ref, cfg, home, destParent, asName)
	}
	m, err := syncRelease(ref, cfg, home, meta)
	if err != nil {
		return "", err
	}
	name := m.Name
	if asName != "" {
		name = asName
	}
	dest := filepath.Join(destParent, name)
	if _, err := os.Lstat(dest); err == nil {
		return "", &packages.ErrAlreadyImported{Name: name, Dest: dest, Digest: m.PackageDigest}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check destination %s: %w", dest, err)
	}
	if err := snapshot.MaterializeRelease(home, m.Name, m.Version, dest); err != nil {
		return "", fmt.Errorf("materialize %s: %w", ref, err)
	}
	return name, nil
}

// ImportArchive preserves the current archive import behavior while also
// recording a complete local release for later offline resolution.
func ImportArchive(r io.Reader, home, destParent, asName string) (string, error) {
	name, err := packages.Import(r, destParent, asName)
	if err != nil {
		return "", err
	}
	if _, err := snapshot.MigrateLegacyInstall(home, filepath.Join(destParent, name)); err != nil {
		_ = os.RemoveAll(filepath.Join(destParent, name))
		return "", fmt.Errorf("record imported package in content store: %w", err)
	}
	return name, nil
}

// Sync obtains a complete release reference without writing a legacy package
// directory. It is the update primitive used by future update commands.
func Sync(ref string, cfg packages.RegistryConfig, home string) (snapshot.ContentManifest, error) {
	meta, err := fetchMetadata(ref, cfg)
	if err != nil {
		return snapshot.ContentManifest{}, err
	}
	return syncRelease(ref, cfg, home, meta)
}

func syncRelease(ref string, cfg packages.RegistryConfig, home string, meta metadata) (snapshot.ContentManifest, error) {
	m := meta.ContentManifest
	if m == nil {
		return snapshot.ContentManifest{}, fmt.Errorf("registry response for %s does not support content-manifest sync", ref)
	}
	if err := snapshot.ValidateContentManifest(*m); err != nil {
		return snapshot.ContentManifest{}, fmt.Errorf("invalid content manifest for %s: %w", ref, err)
	}
	if meta.Name != m.Name || meta.Version != m.Version || meta.Digest != m.PackageDigest {
		return snapshot.ContentManifest{}, fmt.Errorf("registry metadata for %s does not match its content manifest", ref)
	}
	if err := validateObjectPathTemplate(meta.ObjectPathTemplate); err != nil {
		return snapshot.ContentManifest{}, fmt.Errorf("registry response for %s has invalid object path template: %w", ref, err)
	}
	seen := make(map[snapshot.ObjectID]struct{}, len(m.Assets))
	for _, asset := range m.Assets {
		if _, done := seen[asset.Object]; done {
			continue
		}
		seen[asset.Object] = struct{}{}
		status, err := snapshot.ObjectAvailability(home, asset.Object)
		if err != nil {
			return snapshot.ContentManifest{}, fmt.Errorf("inspect cached object %s: %w", asset.Object, err)
		}
		if status == snapshot.ObjectVerified {
			continue
		}
		if status == snapshot.ObjectCorrupt {
			return snapshot.ContentManifest{}, fmt.Errorf("cached object %s is corrupt", asset.Object)
		}
		data, err := fetchObject(cfg, meta.ObjectPathTemplate, asset)
		if err != nil {
			return snapshot.ContentManifest{}, err
		}
		if got := snapshot.ObjectIDFor(data); got != asset.Object {
			return snapshot.ContentManifest{}, fmt.Errorf("downloaded object for %s hashes to %s, want %s", asset.Path, got, asset.Object)
		}
		if _, err := snapshot.WriteObject(home, data); err != nil {
			return snapshot.ContentManifest{}, fmt.Errorf("store downloaded object for %s: %w", asset.Path, err)
		}
	}
	if err := snapshot.CommitRelease(home, *m); err != nil {
		return snapshot.ContentManifest{}, fmt.Errorf("commit %s: %w", ref, err)
	}
	return *m, nil
}

func pullArchive(ref string, cfg packages.RegistryConfig, home, destParent, asName string) (string, error) {
	name, err := packages.Pull(ref, cfg, destParent, asName)
	if err != nil {
		return "", err
	}
	if _, err := snapshot.MigrateLegacyInstall(home, filepath.Join(destParent, name)); err != nil {
		_ = os.RemoveAll(filepath.Join(destParent, name))
		return "", fmt.Errorf("record pulled package in content store: %w", err)
	}
	return name, nil
}

func fetchMetadata(ref string, cfg packages.RegistryConfig) (metadata, error) {
	resp, err := registryClient().Get(cfg.BaseURL() + "/api/packages/" + url.PathEscape(ref))
	if err != nil {
		return metadata{}, fmt.Errorf("resolve %s: %w", ref, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return metadata{}, fmt.Errorf("read metadata for %s: %w", ref, err)
	}
	if resp.StatusCode != http.StatusOK {
		return metadata{}, fmt.Errorf("resolve %s failed (%s): %s", ref, resp.Status, strings.TrimSpace(string(data)))
	}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return metadata{}, fmt.Errorf("parse metadata for %s: %w", ref, err)
	}
	return meta, nil
}

func fetchObject(cfg packages.RegistryConfig, template string, asset snapshot.ContentAsset) ([]byte, error) {
	if asset.Bytes > maxObjectDownloadSize {
		return nil, fmt.Errorf("object for %s is %d bytes, over the %d byte limit", asset.Path, asset.Bytes, maxObjectDownloadSize)
	}
	objectPath := strings.ReplaceAll(template, "{digest}", url.PathEscape(string(asset.Object)))
	resp, err := registryClient().Get(cfg.BaseURL() + objectPath)
	if err != nil {
		return nil, fmt.Errorf("download object for %s: %w", asset.Path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download object for %s failed (%s): %s", asset.Path, resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, asset.Bytes+1))
	if err != nil {
		return nil, fmt.Errorf("read object for %s: %w", asset.Path, err)
	}
	if int64(len(data)) != asset.Bytes {
		return nil, fmt.Errorf("download object for %s has %d bytes, want %d", asset.Path, len(data), asset.Bytes)
	}
	return data, nil
}

func validateObjectPathTemplate(template string) error {
	if !strings.HasPrefix(template, "/") || strings.HasPrefix(template, "//") || !strings.Contains(template, "{digest}") {
		return fmt.Errorf("must be a same-origin absolute path containing {digest}")
	}
	return nil
}

func registryClient() *http.Client {
	return &http.Client{Timeout: registryRequestTimeout}
}
