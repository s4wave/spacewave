//go:build linux

package boot

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

// btrfsIocResize is BTRFS_IOC_RESIZE, _IOW(0x94, 3, struct btrfs_ioctl_vol_args).
const btrfsIocResize = 0x50009403

// btrfsVolArgs is struct btrfs_ioctl_vol_args.
type btrfsVolArgs struct {
	// fd is unused by resize.
	fd int64
	// name holds the NUL-terminated resize amount.
	name [4088]byte
}

// PrepareBackingStore loads at most sixteen matching, dependency-ordered kernel
// modules from this experiment's initramfs, then optionally mounts /dev/vda.
// Module files are kernel content. This path never invokes another executable.
func PrepareBackingStore(persistent, configuration bool) error {
	// A configuration disk is attached only alongside the persistent system.
	if configuration && !persistent {
		return errors.New("configuration requires persistent backing")
	}

	// Packaging prefixes modules with their dependency order. ReadDir sorts
	// them.
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

	// FUSE may be built into the kernel. Persistent boots also need
	// virtio/Btrfs.
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
	if err := growBackingStore("/store"); err != nil {
		return err
	}

	// Configuration uses a distinct raw disk and native data Volume.
	if configuration {
		if err := os.Mkdir("/config-store", 0o700); err != nil {
			return err
		}
		if err := unix.Mount("/dev/vdb", "/config-store", "btrfs", unix.MS_NOATIME, ""); err != nil {
			return err
		}
		println("SPACEWAVE_OS_BACKING_MOUNTED device=/dev/vdb fs=btrfs")
	}
	return nil
}

// growBackingStore grows the Btrfs filesystem mounted at path to fill its
// device, so an image written at its content's size fills the disk it was
// installed on. A filesystem that already fills its device is unchanged, so
// every boot runs growBackingStore.
func growBackingStore(path string) error {
	// Resize the mounted filesystem's first device to its maximum.
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	args := &btrfsVolArgs{}
	copy(args.name[:], "max")
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, dir.Fd(), btrfsIocResize, uintptr(unsafe.Pointer(args))) //nolint:gosec // A fixed-size ioctl argument.
	err = dir.Close()
	if errno != 0 {
		return errors.Join(errors.New("grow "+path), errno)
	}
	if err != nil {
		return err
	}

	// Report the size the data Volume's reserve sees.
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return err
	}
	println("SPACEWAVE_OS_BACKING_GROWN path=" + path + " bytes=" + strconv.FormatUint(st.Blocks*uint64(st.Bsize), 10)) //nolint:gosec // Bsize is positive.
	return nil
}
