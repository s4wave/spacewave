//go:build linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// flushes returns the linux flush variants.
func flushes() []flush {
	return []flush{
		{"fdatasync", func(f *os.File) error { return unix.Fdatasync(int(f.Fd())) }},
		{"fsync", func(f *os.File) error { return unix.Fsync(int(f.Fd())) }},
	}
}

// punch deallocates length bytes at off and keeps the file size.
func punch(f *os.File, off, length int64) error {
	return unix.Fallocate(int(f.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, length)
}
