//go:build windows || js

package unixfs_sync

import "github.com/s4wave/spacewave/db/unixfs"

// applyXattrs is a no-op on Windows.
func applyXattrs(path string, xattrs []unixfs.FSXattr) error {
	return nil
}
