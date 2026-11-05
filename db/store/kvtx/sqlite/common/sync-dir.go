//go:build !windows

package common

import (
	"os"
	"path/filepath"
)

// syncDir flushes the directory holding path.
func syncDir(path string) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
