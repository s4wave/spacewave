package s4db

import (
	"os"

	"golang.org/x/sys/windows"
)

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

// writeback flushes the whole file durably: FlushFileBuffers is the one call
// that writes a file's pages.
func (f *osFile) writeback([]uint64) error {
	return f.flushDurable()
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
