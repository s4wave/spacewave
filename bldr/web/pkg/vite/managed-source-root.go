//go:build !js

package web_pkg_vite

import (
	"os"
	"path/filepath"
	"strings"
)

// managedSourceRoot identifies one compiler-managed filesystem tree.
type managedSourceRoot struct {
	// path is the canonical lexical path to the managed filesystem tree.
	path string
	// info is the filesystem identity of the existing managed root.
	info os.FileInfo
}

func newManagedSourceRoot(codeRootPath, managedRootPath string) (*managedSourceRoot, error) {
	path, err := canonicalSourcePath(codeRootPath, managedRootPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return &managedSourceRoot{path: path, info: info}, nil
}

// Contains reports whether path is inside the managed filesystem tree.
func (r *managedSourceRoot) Contains(path string) bool {
	if r.info != nil {
		for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
			info, err := os.Stat(ancestor)
			if err == nil && os.SameFile(r.info, info) {
				return true
			}
			next := filepath.Dir(ancestor)
			if next == ancestor {
				break
			}
		}
	}

	relPath, err := filepath.Rel(r.path, path)
	return err == nil && relPath != ".." && !strings.HasPrefix(relPath, ".."+string(os.PathSeparator))
}
