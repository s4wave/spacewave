//go:build darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// allocated returns the bytes the file system allocated for path.
func allocated(path string) int64 {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0
	}
	return st.Blocks * 512
}

// evict asks the kernel to drop cached pages of path. Darwin has no per-file
// eviction, so reads after reopen may stay warm.
func evict(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = unix.FcntlInt(f.Fd(), unix.F_NOCACHE, 1)
}

// ioWritten returns the bytes this process wrote to storage.
func ioWritten() int64 {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	// Darwin reports block output operations, not bytes; scale by 4 KiB.
	return ru.Oublock * 4096
}

// rss returns the peak resident set size.
func rss() int64 {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return ru.Maxrss
}
