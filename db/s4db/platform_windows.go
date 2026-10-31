package s4db

import (
	"os"

	"golang.org/x/sys/windows"
)

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

// setThreadPriority is SetThreadPriority, which x/sys/windows leaves out.
var setThreadPriority = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadPriority")

// threadModeBackgroundBegin is THREAD_MODE_BACKGROUND_BEGIN.
const threadModeBackgroundBegin = 0x00010000

// lowerThreadPriority puts the calling thread in background mode, which
// lowers its CPU, disk and memory priority.
func lowerThreadPriority() error {
	if ok, _, err := setThreadPriority.Call(uintptr(windows.CurrentThread()), threadModeBackgroundBegin); ok == 0 {
		return err
	}
	return nil
}
