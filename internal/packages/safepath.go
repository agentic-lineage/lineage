package packages

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeJoin joins rel onto root and guarantees the result stays within root,
// rejecting absolute paths and any ".."-escaping reference however it's
// disguised (e.g. "a/../../etc/passwd"). Use this for every path that comes
// from untrusted, package-controlled input — manifest fields like
// entrypoints, and archive entries during import — never for a path the
// user typed directly at the CLI, which is trusted first-party intent.
func SafeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("unsafe absolute path %q", rel)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	joined := filepath.Join(absRoot, rel)

	if joined != absRoot && !strings.HasPrefix(joined, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes package root", rel)
	}
	return joined, nil
}
