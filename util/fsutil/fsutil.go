package fsutil

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// CleanDir deletes the given dir.
func CleanDir(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

// CleanCreateDir deletes the given dir and then re-creates it.
func CleanCreateDir(path string) error {
	if err := CleanDir(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	return nil
}

// CheckDirEmpty checks if the directory is empty.
func CheckDirEmpty(path string) (bool, error) {
	var anyFiles bool
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if path == "." || path == "" {
			return nil
		}
		if err != nil {
			return err
		}
		anyFiles = true
		return io.EOF
	})
	if anyFiles {
		return false, nil
	}
	if err == io.EOF {
		return false, nil
	}
	return false, err
}

// ConvertPathsToRelative converts a list of paths to relative.
// Enforces that none of the paths are below the base dir.
// Deduplicates the list of paths.
func ConvertPathsToRelative(baseDir string, paths []string) error {
	var err error
	for i := range paths {
		if filepath.IsAbs(paths[i]) {
			paths[i], err = filepath.Rel(baseDir, paths[i])
			if err != nil {
				return err
			}
		}
		paths[i] = filepath.Clean(paths[i])
		if strings.HasPrefix(paths[i], "..") {
			return errors.Errorf("path cannot be above the base dir: %s", paths[i])
		}
	}
	return nil
}
