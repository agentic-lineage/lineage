package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/agentic-lineage/lineage/internal/config"
	"github.com/klauspost/compress/zstd"
)

const (
	compressedObjectNamespace = "zstd-v1"
	minCompressedObjectBytes  = 128
	// Registry object downloads are already capped at 50 MiB. Keeping the
	// compressed representation within that boundary also gives the decoder a
	// firm expansion limit; larger local objects remain supported as raw files.
	maxCompressedObjectBytes = 50 << 20
	minCompressionSavings    = 16
)

var (
	objectEncoderOnce sync.Once
	objectEncoder     *zstd.Encoder
	objectEncoderErr  error
	objectDecoderOnce sync.Once
	objectDecoder     *zstd.Decoder
	objectDecoderErr  error
)

// StoredObjectInfo describes the physical representation of a verified CAS
// object. LogicalBytes always refers to the original bytes addressed by its
// digest; StoredBytes is the on-disk representation size.
type StoredObjectInfo struct {
	Encoding     string
	LogicalBytes int64
	StoredBytes  int64
}

// putObject stores source bytes under their raw digest, choosing compression
// only when it produces a meaningful reduction. The representation namespace
// is separate from identity, so changing encoder output cannot change IDs.
func putObject(root string, data []byte) (ObjectID, error) {
	id := hashID(data)
	if _, _, err := getObject(root, id); err == nil {
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	stored, encoding, err := encodeObject(data)
	if err != nil {
		return "", err
	}
	objectPath, err := objectRepresentationPath(root, id, encoding)
	if err != nil {
		return "", err
	}
	if err := writeBlobNoClobber(objectPath, stored); err != nil {
		return "", err
	}
	if _, _, err := getObject(root, id); err != nil {
		return "", err
	}
	return id, nil
}

func encodeObject(data []byte) ([]byte, string, error) {
	if len(data) < minCompressedObjectBytes || len(data) > maxCompressedObjectBytes {
		return data, "raw", nil
	}
	encoder, err := sharedObjectEncoder()
	if err != nil {
		return nil, "", err
	}
	compressed := encoder.EncodeAll(data, make([]byte, 0, len(data)))
	requiredSavings := max(minCompressionSavings, (len(data)+9)/10)
	if len(compressed) > len(data)-requiredSavings {
		return data, "raw", nil
	}
	return compressed, "zstd", nil
}

func sharedObjectEncoder() (*zstd.Encoder, error) {
	objectEncoderOnce.Do(func() {
		objectEncoder, objectEncoderErr = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedDefault),
			zstd.WithEncoderConcurrency(1),
			zstd.WithEncoderCRC(true),
		)
	})
	return objectEncoder, objectEncoderErr
}

func sharedObjectDecoder() (*zstd.Decoder, error) {
	objectDecoderOnce.Do(func() {
		objectDecoder, objectDecoderErr = zstd.NewReader(nil,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxMemory(maxCompressedObjectBytes),
			zstd.WithDecoderMaxWindow(maxCompressedObjectBytes),
		)
	})
	return objectDecoder, objectDecoderErr
}

func objectRepresentationPath(root string, id ObjectID, encoding string) (string, error) {
	switch encoding {
	case "raw":
		return blobPath(root, id)
	case "zstd":
		return blobPath(filepath.Join(root, compressedObjectNamespace), id)
	default:
		return "", fmt.Errorf("unsupported object encoding %q", encoding)
	}
}

// getObject prefers the legacy raw representation when both exist. An invalid
// raw object is never hidden by a valid compressed copy: corruption remains an
// explicit error instead of being treated as a cache miss.
func getObject(root string, id ObjectID) ([]byte, StoredObjectInfo, error) {
	rawPath, err := objectRepresentationPath(root, id, "raw")
	if err != nil {
		return nil, StoredObjectInfo{}, err
	}
	if _, err := os.Lstat(rawPath); err == nil {
		data, err := getBlob(root, id)
		if err != nil {
			return nil, StoredObjectInfo{}, err
		}
		return data, StoredObjectInfo{Encoding: "raw", LogicalBytes: int64(len(data)), StoredBytes: int64(len(data))}, nil
	} else if !os.IsNotExist(err) {
		return nil, StoredObjectInfo{}, err
	}

	compressedPath, err := objectRepresentationPath(root, id, "zstd")
	if err != nil {
		return nil, StoredObjectInfo{}, err
	}
	info, err := os.Lstat(compressedPath)
	if err != nil {
		return nil, StoredObjectInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, StoredObjectInfo{}, fmt.Errorf("object path %s is not a regular file", compressedPath)
	}
	compressed, err := os.ReadFile(compressedPath)
	if err != nil {
		return nil, StoredObjectInfo{}, err
	}
	decoder, err := sharedObjectDecoder()
	if err != nil {
		return nil, StoredObjectInfo{}, err
	}
	data, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		return nil, StoredObjectInfo{}, fmt.Errorf("object %s has invalid zstd content: %w", id, err)
	}
	if got := hashID(data); got != id {
		return nil, StoredObjectInfo{}, fmt.Errorf("object %s is corrupt: content hashes to %s", id, got)
	}
	return data, StoredObjectInfo{Encoding: "zstd", LogicalBytes: int64(len(data)), StoredBytes: info.Size()}, nil
}

// ObjectStorageInfo verifies id and reports how it is represented locally.
// Missing and corrupt objects retain the same classification as
// ObjectAvailability while verified objects include physical byte accounting.
func ObjectStorageInfo(home string, id ObjectID) (ObjectStatus, StoredObjectInfo, error) {
	_, info, err := getObject(config.ObjectsDir(home), id)
	if err == nil {
		return ObjectVerified, info, nil
	}
	if os.IsNotExist(err) {
		return ObjectMissing, StoredObjectInfo{}, nil
	}
	return ObjectCorrupt, StoredObjectInfo{}, err
}
