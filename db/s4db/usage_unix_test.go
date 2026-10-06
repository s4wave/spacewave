//go:build darwin || linux

package s4db

import (
	"os"
	"syscall"
	"testing"
)

// usage returns the allocated bytes and length of the file at path.
func usage(t testing.TB, path string) (used, size int64) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Sys().(*syscall.Stat_t).Blocks * 512, info.Size()
}
