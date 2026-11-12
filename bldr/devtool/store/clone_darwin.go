//go:build darwin

package devtool_store

import "golang.org/x/sys/unix"

// cloneFile makes dst a copy-on-write clone of src with clonefile(2), which
// shares src's blocks and takes constant time. It fails when src and dst are
// on different volumes or the filesystem cannot clone, and leaves no dst then.
func cloneFile(src, dst string) error {
	return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
}
