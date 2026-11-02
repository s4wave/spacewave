//go:build linux

package boot

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// PrepareBackingStore loads at most sixteen matching, dependency-ordered kernel
// modules from this experiment's initramfs, then optionally mounts /dev/vda.
// Module files are kernel content; this path never invokes another executable.
func PrepareBackingStore(persistent bool) error {
	// Packaging prefixes modules with their dependency order; ReadDir sorts them.
	modules, err := os.ReadDir("/modules")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(modules) > 16 {
		return errors.New("experiment module limit exceeded")
	}
	for _, entry := range modules {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".ko" {
			return errors.New("expected dependency-ordered uncompressed kernel modules")
		}
		file, err := os.Open(filepath.Join("/modules", entry.Name()))
		if err != nil {
			return err
		}
		err = unix.FinitModule(int(file.Fd()), "", 0)
		err = errors.Join(err, file.Close())
		if err != nil {
			return errors.Join(errors.New(entry.Name()), err)
		}
	}

	// FUSE may be built into the kernel; persistent boots also need virtio/Btrfs.
	if _, err := os.Stat("/dev/fuse"); err != nil {
		return err
	}
	if !persistent {
		return nil
	}
	if err := os.Mkdir("/store", 0o700); err != nil {
		return err
	}
	if err := unix.Mount("/dev/vda", "/store", "btrfs", unix.MS_NOATIME, ""); err != nil {
		return err
	}
	println("SPACEWAVE_OS_BACKING_MOUNTED device=/dev/vda fs=btrfs")
	return nil
}
