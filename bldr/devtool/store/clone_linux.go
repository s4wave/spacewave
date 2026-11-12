//go:build linux

package devtool_store

import (
	"os"

	"golang.org/x/sys/unix"
)

// cloneFile makes dst a copy-on-write clone of src with the FICLONE ioctl,
// which shares src's blocks on a filesystem with reflinks, such as btrfs or
// XFS. It fails across filesystems and on ext4 or tmpfs, and leaves no dst then.
func cloneFile(src, dst string) error {
	// Open the source and create the clone's file.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	// Share the source's blocks, removing the file when that fails.
	err = unix.IoctlFileClone(int(out.Fd()), int(in.Fd()))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}
