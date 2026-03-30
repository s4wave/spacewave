//go:build !windows && !js

package unixfs_block

import (
	"golang.org/x/sys/unix"
)

// readFileXattrs reads extended attributes from a filesystem path.
// Returns nil with no error if the path has no xattrs or on error
// (xattrs are best-effort, filesystem may not support them).
// Filters transient macOS xattrs that should not be preserved.
func readFileXattrs(path string) ([]*FSXattr, error) {
	sz, err := unix.Llistxattr(path, nil)
	if err != nil || sz <= 0 {
		return nil, nil
	}
	buf := make([]byte, sz)
	sz, err = unix.Llistxattr(path, buf)
	if err != nil || sz <= 0 {
		return nil, nil
	}
	buf = buf[:sz]

	// Parse null-terminated name list.
	var xattrs []*FSXattr
	for len(buf) > 0 {
		idx := 0
		for idx < len(buf) && buf[idx] != 0 {
			idx++
		}
		if idx == 0 {
			buf = buf[1:]
			continue
		}
		name := string(buf[:idx])
		if idx < len(buf) {
			buf = buf[idx+1:]
		} else {
			buf = nil
		}
		if isTransientXattr(name) {
			continue
		}
		vsz, err := unix.Lgetxattr(path, name, nil)
		if err != nil || vsz < 0 {
			continue
		}
		val := make([]byte, vsz)
		vsz, err = unix.Lgetxattr(path, name, val)
		if err != nil {
			continue
		}
		xattrs = append(xattrs, &FSXattr{Name: name, Value: val[:vsz]})
	}
	return xattrs, nil
}

// transientXattrs are macOS xattrs that should not be preserved.
// Matches the filter list in unixfs/sync/xattr.go.
var transientXattrs = map[string]bool{
	"com.apple.quarantine":                 true,
	"com.apple.lastuseddate#PS":            true,
	"com.apple.metadata:kMDItemWhereFroms": true,
}

// isTransientXattr returns true if the xattr should be skipped during capture.
func isTransientXattr(name string) bool {
	return transientXattrs[name]
}
