//go:build linux

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"

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

// evict drops the cached pages of path.
func evict(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
}

// ioWritten returns the bytes this process caused to be sent to storage.
func ioWritten() int64 {
	return procField("/proc/self/io", "write_bytes:")
}

// rss returns the peak resident set size.
func rss() int64 {
	return procField("/proc/self/status", "VmHWM:") * 1024
}

// procField reads the first number after label in a proc file.
func procField(path, label string) int64 {
	// Scan the lines for the label.
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), label); ok {
			n, _ := strconv.ParseInt(strings.Fields(rest)[0], 10, 64)
			return n
		}
	}
	return 0
}
