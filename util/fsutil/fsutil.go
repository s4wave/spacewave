package fsutil

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CleanCreateDir deletes the given dir and then re-creates it.
func CleanCreateDir(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
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
