package common

import (
	"errors"
	"io/fs"
	"os"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
)

// LocalWAL flushes the WAL of one Store's database file on the local file
// system.
//
// Invariant: a directory flush is owed once per creation or rename of an
// entry in it, and a commit's durability comes from the WAL's data flush. So
// Sync flushes the directory only when the WAL it finds is not the file whose
// entry it flushed before. That covers the database file too: SQLite creates
// it before the WAL, so the first Sync to find a WAL, and every Sync after
// SQLite deletes and recreates the WAL, flushes both entries. SQLite neither
// renames the database nor its WAL.
//
// The WAL whose entry was flushed stays open in synced, so its inode cannot
// be reused by a WAL created later at the same path, which would otherwise
// look like the same file and skip the directory flush it owes.
type LocalWAL struct {
	// mtx serializes Sync and Close.
	mtx sync.Mutex
	// synced is the WAL whose directory entry is durable, nil before the
	// first Sync that found one and after the WAL it named was deleted.
	synced *os.File
}

// Sync makes durable every commit the WAL of the database file at dbPath
// holds, and the directory entries of the database and its WAL. An empty
// path, or a database with no WAL file, has nothing to flush.
func (w *LocalWAL) Sync(dbPath string) error {
	// Skip a database with no file.
	if dbPath == "" {
		return nil
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Flush the WAL whose entry is durable when the path still names it.
	walPath := dbPath + "-wal"
	info, err := os.Stat(walPath)
	if errors.Is(err, fs.ErrNotExist) {
		return w.closeLocked()
	}
	if err != nil {
		return err
	}
	if w.synced != nil {
		if held, err := w.synced.Stat(); err == nil && os.SameFile(held, info) {
			return syncFileData(w.synced)
		}
		if err := w.closeLocked(); err != nil {
			return err
		}
	}

	// Flush a WAL created since, then the directory holding its entry.
	f, err := os.OpenFile(walPath, os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := syncFileData(f); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := syncDir(dbPath); err != nil {
		return errors.Join(err, f.Close())
	}

	// Keep it open where holding it leaves SQLite free to delete it.
	if !keepSyncedWAL {
		return f.Close()
	}
	w.synced = f
	return nil
}

// Close closes the WAL Sync kept open.
func (w *LocalWAL) Close() error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.closeLocked()
}

// closeLocked closes synced. The caller holds mtx.
func (w *LocalWAL) closeLocked() error {
	f := w.synced
	w.synced = nil
	if f == nil {
		return nil
	}
	return f.Close()
}

// syncWALAtOpen flushes the WAL of the database file at dbPath, then drops
// its cached pages, before a Store opens the file. A reader opens the WAL
// read-only, which suffices to flush it and drop its pages, and skips it
// where the platform drops no pages, since there the flush alone keeps the
// cached frames. A WAL with no file at the path, such as one a bridge keeps,
// has nothing to flush.
//
// A WAL flush that failed leaves the kernel's page cache holding frames the
// drive may lack, with valid checksums, and the cache outlives the process.
// SQLite recovers the WAL by reading it when the first connection opens, so
// without the drop it would take those frames for committed. The flush makes
// durable whatever pages the cache still holds dirty, so the drop keeps them,
// and the reads after it see what the drive holds.
//
// The flush runs under stop, the file's fail-stop state, as one of the file's
// flushes, and its failure fails the file.
func syncWALAtOpen(dbPath string, readOnly bool, stop *kvtx.FailStop) error {
	// Open the WAL, which may not exist, read-only for a reader.
	flag := os.O_RDWR
	if readOnly {
		if !dropsCachedPages {
			return nil
		}
		flag = os.O_RDONLY
	}
	f, err := os.OpenFile(dbPath+"-wal", flag, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	// Flush it, then drop its cached pages, as one of the file's flushes.
	err = stop.Flushing(func() error {
		if err := syncFileData(f); err != nil {
			return err
		}
		return dropCachedPages(f)
	}, nil)
	return errors.Join(err, f.Close())
}
