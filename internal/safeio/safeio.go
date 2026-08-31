// Package safeio provides repository-scoped filesystem reads.
package safeio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileWithin reads a repository-relative file only when its fully
// resolved path remains below root. Internal symlinks are allowed; symlinks
// escaping the repository are rejected before the target is read.
func ReadFileWithin(root, rel string) ([]byte, error) {
	if filepath.IsAbs(rel) {
		return nil, fmt.Errorf("absolute path is outside repository scope")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes repository")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, err
	}
	realFile, err := filepath.EvalSymlinks(filepath.Join(rootAbs, clean))
	if err != nil {
		return nil, err
	}
	fromRoot, err := filepath.Rel(realRoot, realFile)
	if err != nil {
		return nil, err
	}
	if filepath.IsAbs(fromRoot) || fromRoot == ".." || strings.HasPrefix(fromRoot, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("symlink resolves outside repository")
	}
	return os.ReadFile(realFile)
}
