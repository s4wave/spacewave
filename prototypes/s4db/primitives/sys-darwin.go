//go:build darwin

package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// flushes returns the darwin flush variants: fsync only reaches the drive's
// cache, F_BARRIERFSYNC orders writes, and F_FULLFSYNC flushes the drive.
func flushes() []flush {
	return []flush{
		{"fsync", func(f *os.File) error { return unix.Fsync(int(f.Fd())) }},
		{"F_BARRIERFSYNC", func(f *os.File) error {
			_, err := unix.FcntlInt(f.Fd(), unix.F_BARRIERFSYNC, 0)
			return err
		}},
		{"F_FULLFSYNC", func(f *os.File) error {
			_, err := unix.FcntlInt(f.Fd(), unix.F_FULLFSYNC, 0)
			return err
		}},
	}
}

// fpunchhole is struct fpunchhole from sys/fcntl.h.
type fpunchhole struct {
	flags    uint32
	reserved uint32
	offset   int64
	length   int64
}

// punch deallocates length bytes at off with F_PUNCHHOLE.
func punch(f *os.File, off, length int64) error {
	arg := fpunchhole{offset: off, length: length}
	_, _, errno := unix.Syscall(unix.SYS_FCNTL, f.Fd(), unix.F_PUNCHHOLE, uintptr(unsafe.Pointer(&arg)))
	if errno != 0 {
		return errno
	}
	return nil
}
