package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// sqliteLockProbes are the byte ranges SQLite's unix VFS locks with fcntl: in
// the main file the pending byte, the reserved byte and the 510 shared bytes
// from offset 0x40000000; in the -shm file the eight WAL index lock slots from
// offset 120 and the dead man switch byte at 128, which every connection holds
// a read lock on for as long as it has the WAL index open.
var sqliteLockProbes = []struct {
	suffix   string
	start, n int64
}{
	{"", 0x40000000, 512},
	{"-shm", 120, 9},
}

// refuseLiveLock returns an error when another connection holds an fcntl lock
// on a SQLite file or its -shm file. It only tests (F_OFD_GETLK on Linux), so
// it takes no lock. It opens and closes the files, which releases any POSIX
// lock this process holds on them: call it before this process opens them.
func refuseLiveLock(path string) error {
	for _, p := range sqliteLockProbes {
		// A missing -shm file holds no lock.
		file := path + p.suffix
		f, err := os.Open(file)
		if errors.Is(err, fs.ErrNotExist) && p.suffix != "" {
			continue
		}
		if err != nil {
			return err
		}

		// Ask whether a write lock over the range would conflict.
		lk := unix.Flock_t{Type: unix.F_WRLCK, Whence: io.SeekStart, Start: p.start, Len: p.n}
		err = unix.FcntlFlock(f.Fd(), getLockCmd, &lk)
		f.Close()
		if err != nil {
			return fmt.Errorf("lock probe of %s: %w", file, err)
		}

		// Any lock there is a live connection, such as a running backend.
		if lk.Type != unix.F_UNLCK {
			kind := "read"
			if lk.Type == unix.F_WRLCK {
				kind = "write"
			}
			return fmt.Errorf("%s: another process holds a %s lock on bytes %d+%d (pid %d, -1 for an open file description lock); a live connection, such as the backend, has the file open: stop it first", file, kind, lk.Start, lk.Len, lk.Pid)
		}
	}
	return nil
}
