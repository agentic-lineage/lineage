package snapshot

import (
	"fmt"
	"strings"
)

// WeightReport keeps package size facts separate from context estimates.
// Bytes come from the canonical content manifest and are exact; context
// counts are reproducible heuristics, never provider-neutral runtime facts.
type WeightReport struct {
	Estimator    EstimatorReport `yaml:"estimator"`
	Assets       []AssetWeight   `yaml:"assets"`
	StoredBytes  int64           `yaml:"stored_bytes"`
	Stub         WeightBucket    `yaml:"stub"`
	FullBody     WeightBucket    `yaml:"full_body"`
	LocalStorage LocalStorage    `yaml:"local_storage"`
}

// EstimatorReport identifies how every available context estimate was made.
type EstimatorReport struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Exact   bool   `yaml:"exact"`
}

// AssetWeight reports one logical package asset. LocalStatus describes the
// CAS object, while Bytes remains the asset's logical package weight.
type AssetWeight struct {
	Path             string          `yaml:"path"`
	Kind             string          `yaml:"kind"`
	Bytes            int64           `yaml:"bytes"`
	LocalStatus      string          `yaml:"local_status"`
	LocalEncoding    string          `yaml:"local_encoding,omitempty"`
	LocalStoredBytes int64           `yaml:"local_stored_bytes,omitempty"`
	Context          ContextEstimate `yaml:"context"`
}

// WeightBucket is an exact byte total plus a context estimate. Available is
// false when a bucket contains content the generic estimator cannot classify.
type WeightBucket struct {
	Bytes   int64           `yaml:"bytes"`
	Context ContextEstimate `yaml:"context"`
}

// ContextEstimate makes unavailable estimates explicit instead of encoding
// them as zero. Tokens is set only when Available is true.
type ContextEstimate struct {
	Available bool   `yaml:"available"`
	Tokens    int64  `yaml:"tokens,omitempty"`
	Reason    string `yaml:"reason,omitempty"`
}

// LocalStorage reports deduplicated CAS accounting. It intentionally differs
// from StoredBytes: two logical assets can point at one immutable object.
type LocalStorage struct {
	VerifiedBytes int64 `yaml:"verified_bytes"`
	PhysicalBytes int64 `yaml:"physical_bytes"`
	SavedBytes    int64 `yaml:"saved_bytes"`
	MissingBytes  int64 `yaml:"missing_bytes"`
	CorruptBytes  int64 `yaml:"corrupt_bytes"`
}

const (
	weightEstimatorName    = "bytes-per-token"
	weightEstimatorVersion = "v1"
	bytesPerEstimatedToken = int64(4)
)

// InspectWeight derives package-weight diagnostics from a validated content
// manifest. It reads the local CAS only to classify availability; it never
// writes objects or a release record, so inspection remains side-effect free.
func InspectWeight(home string, m ContentManifest) (WeightReport, error) {
	if err := validateContentManifest(m); err != nil {
		return WeightReport{}, fmt.Errorf("invalid package content manifest: %w", err)
	}
	report := WeightReport{
		Estimator: EstimatorReport{Name: weightEstimatorName, Version: weightEstimatorVersion},
		Assets:    make([]AssetWeight, 0, len(m.Assets)),
		// A package can legitimately have only its manifest. An empty body is
		// still an exact, estimable zero rather than an unknown estimate.
		Stub:     WeightBucket{Context: ContextEstimate{Available: true}},
		FullBody: WeightBucket{Context: ContextEstimate{Available: true}},
	}
	type inspectedObject struct {
		status ObjectStatus
		info   StoredObjectInfo
	}
	inspected := make(map[ObjectID]inspectedObject, len(m.Assets))
	seen := make(map[ObjectID]struct{}, len(m.Assets))
	for _, asset := range m.Assets {
		context := estimateContext(asset)
		entry := AssetWeight{Path: asset.Path, Kind: asset.Kind, Bytes: asset.Bytes, Context: context}
		local, ok := inspected[asset.Object]
		if !ok {
			status, info, err := ObjectStorageInfo(home, asset.Object)
			if err != nil && status != ObjectCorrupt {
				return WeightReport{}, fmt.Errorf("inspect local object %s: %w", asset.Object, err)
			}
			local = inspectedObject{status: status, info: info}
			inspected[asset.Object] = local
		}
		entry.LocalStatus = localStatusName(local.status)
		if local.status == ObjectVerified {
			entry.LocalEncoding = local.info.Encoding
			entry.LocalStoredBytes = local.info.StoredBytes
		}
		report.Assets = append(report.Assets, entry)
		report.StoredBytes += asset.Bytes
		if asset.Kind == "manifest" {
			addWeight(&report.Stub, asset.Bytes, context)
		} else {
			addWeight(&report.FullBody, asset.Bytes, context)
		}
		if _, duplicate := seen[asset.Object]; duplicate {
			continue
		}
		seen[asset.Object] = struct{}{}
		switch local.status {
		case ObjectVerified:
			report.LocalStorage.VerifiedBytes += local.info.LogicalBytes
			report.LocalStorage.PhysicalBytes += local.info.StoredBytes
			report.LocalStorage.SavedBytes += local.info.LogicalBytes - local.info.StoredBytes
		case ObjectMissing:
			report.LocalStorage.MissingBytes += asset.Bytes
		case ObjectCorrupt:
			report.LocalStorage.CorruptBytes += asset.Bytes
		}
	}
	return report, nil
}

func estimateContext(asset ContentAsset) ContextEstimate {
	if !isTextAsset(asset) {
		return ContextEstimate{Reason: "unavailable for non-text asset"}
	}
	return ContextEstimate{Available: true, Tokens: (asset.Bytes + bytesPerEstimatedToken - 1) / bytesPerEstimatedToken}
}

func isTextAsset(asset ContentAsset) bool {
	return strings.HasPrefix(asset.MediaType, "text/") || asset.MediaType == "application/yaml" || asset.MediaType == "application/json"
}

func addWeight(bucket *WeightBucket, bytes int64, context ContextEstimate) {
	bucket.Bytes += bytes
	if !context.Available {
		bucket.Context = ContextEstimate{Reason: "unavailable because one or more assets are non-text"}
		return
	}
	if bucket.Context.Reason != "" {
		return
	}
	bucket.Context.Available = true
	bucket.Context.Tokens += context.Tokens
}

func localStatusName(status ObjectStatus) string {
	switch status {
	case ObjectVerified:
		return "verified"
	case ObjectCorrupt:
		return "corrupt"
	default:
		return "missing"
	}
}
