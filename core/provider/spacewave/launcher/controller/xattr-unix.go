//go:build !windows && !js

package spacewave_launcher_controller

import (
	"strings"

	"golang.org/x/sys/unix"
)

const paxXattrPrefix = "SCHILY.xattr."

// applyXattrsFromPAX applies extended attributes from tar PAX records to a
// file. PAX records for xattrs use the prefix "SCHILY.xattr.<name>".
func applyXattrsFromPAX(path string, pax map[string]string) {
	for key, val := range pax {
		if !strings.HasPrefix(key, paxXattrPrefix) {
			continue
		}
		name := key[len(paxXattrPrefix):]
		if name == "" || isTransientXattr(name) {
			continue
		}
		// best effort: ignore errors from unsupported xattrs
		_ = unix.Lsetxattr(path, name, []byte(val), 0)
	}
}

// isTransientXattr returns true for macOS xattrs that should not be restored.
func isTransientXattr(name string) bool {
	switch name {
	case "com.apple.quarantine",
		"com.apple.lastuseddate#PS",
		"com.apple.metadata:kMDItemWhereFroms":
		return true
	}
	return false
}
