//go:build !windows

package common

import (
	"os"
	"path/filepath"
)

// keepSyncedWAL keeps the synced WAL open, as LocalWAL describes. A POSIX
// file system deletes an open file's entry while its inode stays reserved.
const keepSyncedWAL = true

// syncDir flushes the directory holding path.
func syncDir(path string) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
