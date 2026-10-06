package s4db

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// zeroData is FILE_ZERO_DATA_INFORMATION.
type zeroData struct {
	// offset is the first byte to deallocate.
	offset int64
	// beyond is the byte after the last one to deallocate.
	beyond int64
}

// lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
// Windows locks belong to the handle, and the system drops them when the
// process exits.
func (f *osFile) lock(off int64, wait bool) (bool, error) {
	// Fail at once on a held lock unless waiting.
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}

	// Take the lock; a violation means another holder has it.
	ov := lockOverlapped(off)
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &ov)
	switch err {
	case nil:
		return true, nil
	case windows.ERROR_LOCK_VIOLATION, windows.ERROR_IO_PENDING:
		return false, nil
	}
	return false, err
}

// unlock releases the lock on the byte at off.
func (f *osFile) unlock(off int64) error {
	ov := lockOverlapped(off)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ov)
}

// held reports whether another holder has the lock on the byte at off.
// Windows cannot query a lock, so held takes and drops it; a process taking
// that slot meanwhile moves on to the next free one.
func (f *osFile) held(off int64) (bool, error) {
	ok, err := f.lock(off, false)
	if err != nil || !ok {
		return !ok, err
	}
	return false, f.unlock(off)
}

// lockOverlapped returns the OVERLAPPED that addresses the byte at off.
func lockOverlapped(off int64) windows.Overlapped {
	return windows.Overlapped{Offset: uint32(off), OffsetHigh: uint32(off >> 32)} // #nosec G115 -- splits off into its halves.
}

// identify returns the volume serial and file index of f.
func identify(f *os.File) (fileID, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return fileID{}, err
	}
	return fileID{
		volume: uint64(info.VolumeSerialNumber),
		index:  uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
	}, nil
}

// flushDurable makes earlier writes durable on the drive.
func (f *osFile) flushDurable() error {
	return windows.FlushFileBuffers(windows.Handle(f.Fd()))
}

// flushOrdered orders earlier writes before later ones. Windows has no write
// barrier short of a full flush, so ordered commits rely on record
// checksums: recovery keeps the longest valid prefix of the log.
func (*osFile) flushOrdered() error {
	return nil
}

// flushBarrier orders earlier writes before later ones. Windows has no
// cheaper ordering than a full flush, so the barrier is durable.
func (f *osFile) flushBarrier() (bool, error) {
	return true, f.flushDurable()
}

// punch deallocates n bytes at off, keeping the file length. The file is
// marked sparse first; marking it again is a no-op.
func (f *osFile) punch(off, n int64) error {
	// Mark the file sparse so zeroed ranges release their clusters.
	h := windows.Handle(f.Fd())
	var done uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, &done, nil); err != nil {
		return err
	}

	// Zero the range.
	zd := zeroData{offset: off, beyond: off + n}
	return windows.DeviceIoControl(h, windows.FSCTL_SET_ZERO_DATA, (*byte)(unsafe.Pointer(&zd)), uint32(unsafe.Sizeof(zd)), nil, 0, &done, nil)
}
