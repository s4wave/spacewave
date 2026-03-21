//go:build !windows

package unixfs_sync

import (
	"github.com/aperturerobotics/hydra/unixfs"
	"golang.org/x/sys/unix"
)

// applyXattrs sets extended attributes on the given path.
// Skips transient macOS xattrs that should not be preserved.
func applyXattrs(path string, xattrs []unixfs.FSXattr) error {
	for _, xa := range xattrs {
		if isTransientXattr(xa.Name) {
			continue
		}
		if err := unix.Lsetxattr(path, xa.Name, xa.Value, 0); err != nil {
			return err
		}
	}
	return nil
}

// transientXattrs are macOS xattrs that should not be preserved during sync.
var transientXattrs = map[string]bool{
	"com.apple.quarantine":              true,
	"com.apple.lastuseddate#PS":         true,
	"com.apple.metadata:kMDItemWhereFroms": true,
}

// isTransientXattr returns true if the xattr should be skipped.
func isTransientXattr(name string) bool {
	return transientXattrs[name]
}
