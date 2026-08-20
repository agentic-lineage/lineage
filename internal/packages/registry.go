package packages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultRegistryURL is used when LINEAGE_REGISTRY_URL isn't set. It points
// at the Lineage website's package API (docs/decisions/
// 0012-v1-distribution-contract-and-receiver-activation.md), which is
// itself a thin proxy in front of a private GitHub repo used as artifact
// storage - the registry API is the only interface this file talks to.
const DefaultRegistryURL = "https://lineage.dev"

// RegistryConfig is read from the environment by CLI commands, not stored
// in .lineage/config.yaml: a publish token is a per-invocation secret, not
// committed project state.
type RegistryConfig struct {
	URL   string
	Token string
}

func (c RegistryConfig) baseURL() string {
	if c.URL != "" {
		return strings.TrimRight(c.URL, "/")
	}
	return DefaultRegistryURL
}

type PublishResult struct {
	Name             string
	Version          string
	Digest           string
	AlreadyPublished bool
}

// Publish validates dir the same way Export does, then uploads the
// resulting archive to the registry. It refuses to run against a package
// that fails Validate for the same reason Export does: publish is exactly
// the moment package content starts moving to other machines.
func Publish(dir string, cfg RegistryConfig) (PublishResult, error) {
	report, err := Validate(dir)
	if err != nil {
		return PublishResult{}, err
	}
	if !report.Passed() {
		return PublishResult{}, fmt.Errorf("package failed validation, refusing to publish (%d error(s)); run `lineage package validate` for details", len(report.Errors))
	}
	if cfg.Token == "" {
		return PublishResult{}, fmt.Errorf("no publish token configured; set LINEAGE_PUBLISH_TOKEN")
	}

	var buf bytes.Buffer
	if err := Export(dir, &buf); err != nil {
		return PublishResult{}, err
	}

	q := url.Values{}
	q.Set("name", report.Manifest.Name)
	q.Set("version", report.Manifest.Version)
	q.Set("digest", report.Digest)
	if report.Manifest.Description != "" {
		q.Set("description", report.Manifest.Description)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.baseURL()+"/api/publish?"+q.Encode(), &buf)
	if err != nil {
		return PublishResult{}, fmt.Errorf("build publish request: %w", err)
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return PublishResult{}, fmt.Errorf("publish %s@%s: %w", report.Manifest.Name, report.Manifest.Version, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return PublishResult{}, fmt.Errorf("publish %s@%s failed (%s): %s", report.Manifest.Name, report.Manifest.Version, resp.Status, apiErrorMessage(body))
	}

	var parsed struct {
		AlreadyPublished bool `json:"alreadyPublished"`
	}
	_ = json.Unmarshal(body, &parsed)

	return PublishResult{
		Name:             report.Manifest.Name,
		Version:          report.Manifest.Version,
		Digest:           report.Digest,
		AlreadyPublished: parsed.AlreadyPublished,
	}, nil
}

type packageMetadata struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Digest       string `json:"digest"`
	Description  string `json:"description"`
	DownloadPath string `json:"downloadPath"`
}

// Pull fetches a published package by ref ("<name>@<version>", or just
// "<name>" for the latest published version) from the registry and imports
// it into destParent, the same way Import does for a local archive.
//
// The digest the registry reports for ref is untrusted the same way the
// archive bytes are: after import, Pull recomputes the digest from the
// content Import actually kept and fails closed - removing what was
// imported - if it doesn't match what the registry claimed.
func Pull(ref string, cfg RegistryConfig, destParent, asName string) (string, error) {
	meta, err := fetchPackageMetadata(ref, cfg)
	if err != nil {
		return "", err
	}

	resp, err := http.Get(cfg.baseURL() + meta.DownloadPath)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", ref, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("download %s failed (%s): %s", ref, resp.Status, apiErrorMessage(body))
	}

	name, err := Import(resp.Body, destParent, asName)
	if err != nil {
		return "", err
	}

	importedDir := filepath.Join(destParent, name)
	actualDigest, err := ComputeDigest(importedDir)
	if err != nil {
		os.RemoveAll(importedDir)
		return "", fmt.Errorf("verify digest for %s: %w", ref, err)
	}
	if meta.Digest != "" && actualDigest != meta.Digest {
		os.RemoveAll(importedDir)
		return "", fmt.Errorf("digest mismatch for %s: registry reported %s, imported content hashes to %s; refusing to keep it", ref, meta.Digest, actualDigest)
	}

	return name, nil
}

func fetchPackageMetadata(ref string, cfg RegistryConfig) (packageMetadata, error) {
	resp, err := http.Get(cfg.baseURL() + "/api/packages/" + url.PathEscape(ref))
	if err != nil {
		return packageMetadata{}, fmt.Errorf("resolve %s: %w", ref, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return packageMetadata{}, fmt.Errorf("read metadata for %s: %w", ref, err)
	}
	if resp.StatusCode != http.StatusOK {
		return packageMetadata{}, fmt.Errorf("resolve %s failed (%s): %s", ref, resp.Status, apiErrorMessage(body))
	}

	var meta packageMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return packageMetadata{}, fmt.Errorf("parse metadata for %s: %w", ref, err)
	}
	return meta, nil
}

// apiErrorMessage extracts the registry's {"error": "..."} body into a
// clean message, falling back to the raw body if it isn't that shape. The
// registry's own error text already carries retry guidance where it
// applies (e.g. "safe to retry the same publish" after an interrupted
// upload) - this just keeps that readable instead of dumping raw JSON.
func apiErrorMessage(body []byte) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != "" {
		return parsed.Error
	}
	return strings.TrimSpace(string(body))
}
