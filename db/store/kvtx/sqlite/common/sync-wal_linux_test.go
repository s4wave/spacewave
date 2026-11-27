//go:build linux

package common

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sys/unix"
)

// TestSyncWALAtOpenDropsPages writes a WAL through the page cache and runs the
// open flush of a writer and of a reader on it. No page of the WAL may stay
// cached: SQLite recovers the WAL by reading it, and a reader reads it, so a
// cached frame whose flush failed reads as committed.
func TestSyncWALAtOpenDropsPages(t *testing.T) {
	// The cache of a file in memory is its storage, which no flush drops.
	dir := t.TempDir()
	var fs unix.Statfs_t
	if err := unix.Statfs(dir, &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Type == unix.TMPFS_MAGIC {
		t.Skip("the temporary directory is on tmpfs; set TMPDIR to a disk")
	}

	dbPath := filepath.Join(dir, "db.sqlite")
	for _, readOnly := range []bool{false, true} {
		// Write a WAL whose pages the cache holds, dirty.
		wal := bytes.Repeat([]byte{0xa5}, 1<<20)
		if err := os.WriteFile(dbPath+"-wal", wal, 0o600); err != nil {
			t.Fatal(err)
		}
		if got := cachedPages(t, dbPath+"-wal"); got == 0 {
			t.Fatal("the written WAL has no cached page to drop")
		}

		// Flush it as an open does. The cache must hold none of its pages.
		if err := syncWALAtOpen(dbPath, readOnly, kvtx.NewFailStop()); err != nil {
			t.Fatal(err)
		}
		if got := cachedPages(t, dbPath+"-wal"); got != 0 {
			t.Fatalf("%d pages of the WAL stay cached after the open flush (read-only %v)", got, readOnly)
		}
	}
}

// cachedPages counts the pages of the file at path the page cache holds.
func cachedPages(t *testing.T, path string) int {
	// Map the file without reading it.
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	m, err := unix.Mmap(int(f.Fd()), 0, int(info.Size()), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Munmap(m) }()

	// Count the pages mincore reports resident. x/sys/unix wraps mincore only
	// on the BSDs.
	page := os.Getpagesize()
	vec := make([]byte, (len(m)+page-1)/page)
	if _, _, errno := unix.Syscall(unix.SYS_MINCORE, uintptr(unsafe.Pointer(&m[0])), uintptr(len(m)), uintptr(unsafe.Pointer(&vec[0]))); errno != 0 {
		t.Fatal(errno)
	}
	n := 0
	for _, v := range vec {
		n += int(v & 1)
	}
	return n
}
