package main

import (
	"os"
	"syscall"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/sqlitefree"
	"golang.org/x/sys/unix"
)

// SQLite's unix VFS lock bytes: PENDING at pendingByte, RESERVED after it,
// then the 510 byte SHARED range. EXCLUSIVE is a write lock on SHARED.
const lockSpan = 2 + 510

// shm lock bytes: the eight WAL locks from 120, then the DMS byte at 128,
// which every connection with the WAL index open holds read-locked.
const shmLockStart, shmLockSpan = 120, 9

// lockExclusive takes write locks on every SQLite lock byte of the database
// file, and on the -shm lock bytes if the file exists. It uses open file
// description (OFD) locks: they conflict with the classic POSIX fcntl locks
// SQLite takes, from any process including this one, and closing some other
// descriptor does not drop them. Any SQLite connection holding SHARED or
// more on the file, or holding the WAL index open (even idle between
// transactions), makes the lock fail; while it is held, no connection can
// begin a read or write transaction.
func lockExclusive(f *os.File, path string) (func(), error) {
	if err := ofdWrite(f, pendingByte, lockSpan); err != nil {
		return nil, errors.Wrap(err, "database is in use (SQLite lock bytes)")
	}
	shm, err := os.OpenFile(path+"-shm", os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return func() {}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "open -shm")
	}
	if err := ofdWrite(shm, shmLockStart, shmLockSpan); err != nil {
		shm.Close()
		return nil, errors.Wrap(err, "database is in use (-shm DMS lock held by an open connection)")
	}
	return func() { shm.Close() }, nil
}

func ofdWrite(f *os.File, start, n int64) error {
	lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: start, Len: n}
	return unix.FcntlFlock(f.Fd(), unix.F_OFD_SETLK, &lk)
}

// punchRanges releases each range's blocks, keeping the file size.
func punchRanges(f *os.File, h header, rs []pageRange) error {
	return sqlitefree.PunchRanges(f, h.PageSize, rs)
}

func allocated(st os.FileInfo) int64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return s.Blocks * 512
	}
	return -1
}

// openDirect opens path read-only with O_DIRECT, so bulk reads skip the
// page cache.
func openDirect(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_DIRECT, 0)
}
